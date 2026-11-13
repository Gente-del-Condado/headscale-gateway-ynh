package portal

import (
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
	"html/template"
	"net/http"
)

var page = template.Must(template.New("portal").Parse(`<!doctype html><html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Config.Title}}</title><link rel="icon" href="/portal/favicon.png"><style>
:root{color-scheme:dark;--bg:#0b1020;--card:#172236;--text:#f1f5f9;--muted:#a5b4ca;--accent:{{.Accent}}}body.light{color-scheme:light;--bg:#f1f5f9;--card:#fff;--text:#172236;--muted:#475569}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:16px/1.5 system-ui}main{max-width:1150px;margin:auto;padding:45px 22px}header{display:flex;align-items:center;gap:20px}header img{max-width:90px;max-height:90px}h1{margin:0;font-size:clamp(2rem,5vw,3rem)}.muted,small{color:var(--muted)}.tools{display:flex;gap:12px;flex-wrap:wrap;margin:28px 0}input,select{padding:13px;border:1px solid var(--muted);border-radius:10px;background:var(--card);color:var(--text);font:inherit}input{flex:1;min-width:200px}.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:16px}a.card{display:block;color:inherit;text-decoration:none;background:var(--card);border:1px solid transparent;border-radius:16px;padding:23px;overflow-wrap:anywhere}.card:hover,.card:focus-visible{border-color:var(--accent)}.card strong{font-size:1.15rem;color:var(--accent)}.card small{display:block}.card p{margin:10px 0}a{color:var(--accent)}[hidden]{display:none!important}footer{margin-top:35px}
</style></head><body class="{{.Config.Theme}}"><main><header>{{if .Config.Logo}}<img src="/portal/logo.png" alt="">{{end}}<div><h1>{{.Config.Title}}</h1><p class="muted">{{.Config.Description}}</p></div></header>
{{if .Config.Search}}<div class="tools"><input id="search" type="search" placeholder="{{.Search}}" aria-label="{{.Search}}"><select id="gateway" aria-label="{{.Gateway}}"><option value="">{{.All}}</option>{{range .Gateways}}<option>{{.}}</option>{{end}}</select></div>{{end}}
<div class="grid">{{range .Services}}<a class="card" href="{{.URL}}" data-gateway="{{.Gateway}}" data-search="{{.Name}} {{.Description}} {{.Group}} {{.Gateway}}" rel="noopener noreferrer"><strong>{{.Name}}</strong><p>{{.Description}}</p><small>{{.Group}}</small><small>{{.Gateway}}</small></a>{{end}}</div><p id="none" hidden>{{.Empty}}</p>
<footer class="muted"><p>{{.Notice}}</p>{{if .Admin}}<a href="{{.Admin}}">{{.Administration}}</a>{{end}}</footer></main>
<script>const s=document.querySelector('#search'),g=document.querySelector('#gateway'),cards=[...document.querySelectorAll('.card')];function filter(){let n=0;cards.forEach(c=>{c.hidden=!!((s&&!c.dataset.search.toLocaleLowerCase().includes(s.value.toLocaleLowerCase()))||(g&&g.value&&g.value!==c.dataset.gateway));if(!c.hidden)n++});document.querySelector('#none').hidden=n>0}if(s)s.oninput=filter;if(g)g.onchange=filter;filter();</script></body></html>`))

func Render(w http.ResponseWriter, r *http.Request, c Config, entries []Entry, admin string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	lang := i18n.Language(r.Header.Get("Accept-Language"))
	tr := func(s string) string { return i18n.Translate(lang, s) }
	if c.Title == "Servicios" {
		c.Title = tr("Servicios")
	}
	if c.Description == "Portal privado de servicios" {
		c.Description = tr("Portal privado de servicios")
	}
	gateways := []string{}
	seen := map[string]bool{}
	for _, e := range entries {
		if !seen[e.Gateway] {
			seen[e.Gateway] = true
			gateways = append(gateways, e.Gateway)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	return page.Execute(w, struct {
		Lang                                                       string
		Config                                                     Config
		Accent                                                     template.CSS
		Services                                                   []Entry
		Gateways                                                   []string
		Admin, Search, Gateway, All, Empty, Notice, Administration string
	}{lang, c, template.CSS(c.Accent), entries, gateways, admin, tr("Buscar servicios"), tr("Gateway"), tr("Todos los gateways"), tr("No hay servicios que coincidan."), tr("Los enlaces remotos no conceden permisos. El acceso depende de tu conexión y de las ACL."), tr("Administración")})
}
