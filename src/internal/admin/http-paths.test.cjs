'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const source = fs.readFileSync(__dirname + '/http-paths.js', 'utf8');
test('standalone, ingress and YunoHost keep API/download paths in their mount', () => {
  for (const [path, root] of [['/admin/', ''], ['/gateway/admin/', '/gateway'], ['/api/hassio_ingress/abc/admin/panel.html', '/api/hassio_ingress/abc']]) {
    const window = {location: {pathname: path}};
    vm.runInNewContext(source, {window});
    assert.equal(window.gatewayURL('/api/v1/status'), root + '/api/v1/status');
    assert.equal(window.gatewayURL('/admin/locales/en.json'), root + '/admin/locales/en.json');
    for (const bad of ['https://evil.test', '//evil.test', '/api/../secret', '/api\\secret']) assert.throws(() => window.gatewayURL(bad));
  }
});
