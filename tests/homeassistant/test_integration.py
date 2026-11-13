from unittest.mock import AsyncMock, patch
from importlib.util import find_spec

from aiohttp import web
import pytest

from homeassistant.config_entries import SOURCE_USER
from homeassistant.data_entry_flow import FlowResultType
from homeassistant.helpers import entity_registry as er
if find_spec('pytest_homeassistant') is not None:
    from pytest_homeassistant.common import MockConfigEntry
else:
    from pytest_homeassistant_custom_component.common import MockConfigEntry

from custom_components.headscale_gateway.api import GatewayAuthError, GatewayClient, GatewayError, normalize_url

pytestmark = pytest.mark.usefixtures('enable_custom_integrations')

STATUS = {'vpn_connected': True, 'lan_https_active': True, 'setup_required': False,
          'services_active': 4, 'uptime_seconds': 123, 'certificate_source': 'local-ca',
          'domain': 'node.example.test'}
DATA = {'host': 'https://gateway.example.test/mount', 'token': 'hgk_test', 'verify_ssl': True}


async def test_setup_entities_and_unload(hass):
    entry = MockConfigEntry(domain='headscale_gateway', data=DATA, unique_id=DATA['host'])
    entry.add_to_hass(hass)
    with patch.object(GatewayClient, 'status', AsyncMock(return_value=STATUS)):
        assert await hass.config_entries.async_setup(entry.entry_id)
        await hass.async_block_till_done()
    registry = er.async_get(hass)
    entities = er.async_entries_for_config_entry(registry, entry.entry_id)
    assert len(entities) == 7
    vpn = next(e for e in entities if e.unique_id.endswith('_vpn_connected'))
    assert hass.states.get(vpn.entity_id).state == 'on'
    uptime = next(e for e in entities if e.unique_id.endswith('_uptime_seconds'))
    assert hass.states.get(uptime.entity_id).state == '123'
    assert 'token' not in str(hass.states.get(vpn.entity_id).attributes)
    assert await hass.config_entries.async_unload(entry.entry_id)


async def test_unavailable_on_connection_failure(hass):
    entry = MockConfigEntry(domain='headscale_gateway', data=DATA, unique_id=DATA['host'])
    entry.add_to_hass(hass)
    with patch.object(GatewayClient, 'status', AsyncMock(return_value=STATUS)):
        assert await hass.config_entries.async_setup(entry.entry_id)
        await hass.async_block_till_done()
    entities = er.async_entries_for_config_entry(er.async_get(hass), entry.entry_id)
    with patch.object(GatewayClient, 'status', AsyncMock(side_effect=GatewayError('offline'))):
        await entry.runtime_data.async_refresh()
        await hass.async_block_till_done()
    assert all(hass.states.get(entity.entity_id).state == 'unavailable' for entity in entities)


async def test_reauth_renews_expired_token(hass):
    entry = MockConfigEntry(domain='headscale_gateway', data=DATA, unique_id=DATA['host'])
    entry.add_to_hass(hass)
    result = await hass.config_entries.flow.async_init('headscale_gateway', context={
        'source': 'reauth', 'entry_id': entry.entry_id}, data=DATA)
    assert result['type'] == FlowResultType.FORM
    with patch.object(GatewayClient, 'status', AsyncMock(return_value=STATUS)):
        result = await hass.config_entries.flow.async_configure(result['flow_id'], dict(DATA, token='hgk_renewed'))
        await hass.async_block_till_done()
    assert result['type'] == FlowResultType.ABORT
    assert result['reason'] == 'reauth_successful'
    assert entry.data['token'] == 'hgk_renewed'


async def test_config_flow_and_duplicate(hass):
    with patch.object(GatewayClient, 'status', AsyncMock(return_value=STATUS)):
        result = await hass.config_entries.flow.async_init('headscale_gateway', context={'source': SOURCE_USER}, data=DATA)
        assert result['type'] == FlowResultType.CREATE_ENTRY
        await hass.async_block_till_done()
        result = await hass.config_entries.flow.async_init('headscale_gateway', context={'source': SOURCE_USER}, data=DATA)
        assert result['type'] == FlowResultType.ABORT
        assert result['reason'] == 'already_configured'


async def test_bad_token_does_not_create_entry(hass):
    with patch.object(GatewayClient, 'status', AsyncMock(side_effect=GatewayAuthError('denied'))):
        result = await hass.config_entries.flow.async_init('headscale_gateway', context={'source': SOURCE_USER}, data=DATA)
    assert result['type'] == FlowResultType.FORM
    assert result['errors'] == {'base': 'invalid_auth'}


async def test_reconfigure_renews_credentials(hass):
    entry = MockConfigEntry(domain='headscale_gateway', data=DATA, unique_id=DATA['host'])
    entry.add_to_hass(hass)
    updated = dict(DATA, token='hgk_replacement')
    with patch.object(GatewayClient, 'status', AsyncMock(return_value=STATUS)):
        result = await hass.config_entries.flow.async_init('headscale_gateway', context={'source': 'reconfigure', 'entry_id': entry.entry_id}, data=updated)
        await hass.async_block_till_done()
    assert result['type'] == FlowResultType.ABORT
    assert entry.data['token'] == 'hgk_replacement'


@pytest.mark.parametrize('url', ['http://192.168.1.2', 'https://user:password@example.test', 'https://example.test/?token=secret', 'file:///etc/passwd'])
def test_reject_unsafe_url(url):
    with pytest.raises(GatewayError):
        normalize_url(url)


async def test_api_redirect_and_auth_are_not_followed():
    response = AsyncMock()
    response.__aenter__.return_value = response
    response.status = 302
    session = AsyncMock()
    # ClientSession.get returns an async context manager, not a coroutine.
    from unittest.mock import Mock
    session.get = Mock(return_value=response)
    client = GatewayClient(session, DATA['host'], 'hgk_test')
    with pytest.raises(GatewayError):
        await client.status()
    assert session.get.call_args.kwargs['allow_redirects'] is False
    response.status = 401
    with pytest.raises(GatewayAuthError):
        await client.status()


async def test_http_setup_poll_failure_recovery_and_reauth(hass, aiohttp_server, socket_enabled):
    """Exercise Core and the real HTTP client, including automatic reauthentication."""
    state = {'token': 'hgk_first', 'status': 200, 'calls': 0}

    async def status(request):
        state['calls'] += 1
        assert request.headers.get('Authorization') == 'Bearer ' + state['token']
        if state['status'] != 200:
            return web.Response(status=state['status'])
        return web.json_response(dict(STATUS, version='test', services_active=state['calls'],
                                      future_secret='must-not-reach-entities'))

    app = web.Application()
    app.router.add_get('/gateway/api/v1/status', status)
    server = await aiohttp_server(app, host='127.0.0.1')
    data = dict(DATA, host=str(server.make_url('/gateway')), token=state['token'])
    result = await hass.config_entries.flow.async_init(
        'headscale_gateway', context={'source': SOURCE_USER}, data=data)
    assert result['type'] == FlowResultType.CREATE_ENTRY
    await hass.async_block_till_done()
    entry = result['result']
    entities = er.async_entries_for_config_entry(er.async_get(hass), entry.entry_id)
    assert len(entities) == 7
    assert all(hass.states.get(e.entity_id).state != 'unavailable' for e in entities)
    services = next(e for e in entities if e.unique_id.endswith('_services_active'))
    assert hass.states.get(services.entity_id).state == str(state['calls'])
    assert 'future_secret' not in entry.runtime_data.data

    state['status'] = 503
    await entry.runtime_data.async_refresh()
    await hass.async_block_till_done()
    assert all(hass.states.get(e.entity_id).state == 'unavailable' for e in entities)
    state['status'] = 200
    await entry.runtime_data.async_refresh()
    await hass.async_block_till_done()
    assert hass.states.get(services.entity_id).state == str(state['calls'])

    state['status'] = 401
    await entry.runtime_data.async_refresh()
    await hass.async_block_till_done()
    flows = [flow for flow in hass.config_entries.flow.async_progress()
             if flow['handler'] == 'headscale_gateway' and flow['context']['source'] == 'reauth']
    assert len(flows) == 1
    state.update(status=200, token='hgk_renewed')
    result = await hass.config_entries.flow.async_configure(
        flows[0]['flow_id'], dict(data, token=state['token']))
    await hass.async_block_till_done()
    assert result['reason'] == 'reauth_successful'
    assert entry.data['token'] == state['token']
    assert {e.unique_id for e in er.async_entries_for_config_entry(er.async_get(hass), entry.entry_id)} == {e.unique_id for e in entities}
    assert all(hass.states.get(e.entity_id).state != 'unavailable' for e in entities)
    assert await hass.config_entries.async_unload(entry.entry_id)
