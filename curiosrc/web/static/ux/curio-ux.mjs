import {LitElement, css, html} from 'https://cdn.jsdelivr.net/gh/lit/dist@3/all/lit-all.min.js';

//import 'https://cdn.jsdelivr.net/npm/bootstrap@5.1.3/dist/js/bootstrap.esm.js';


class CurioUX extends LitElement {
  static styles = css`
\  .curio-slot {
  }
  :host {
    display: block;
    margin: 2px 3px;
  }
  
  `;
  connectedCallback() { 
    super.connectedCallback(); 
      //"https://unpkg.com/@cds/core/global.min.css",
      //"https://unpkg.com/@cds/city/css/bundles/default.min.css",
      //"https://unpkg.com/@cds/core/styles/theme.dark.min.css",
      //"https://unpkg.com/@clr/ui/clr-ui.min.css",

    document.head.innerHTML += `
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.1.3/dist/css/bootstrap.min.css" rel="stylesheet" 
      integrity="sha384-1BmE4kWBq78iYhFldvKuhfTAU6auU8tT94WrHftjDbrCEXSU1oBoqyl2QvZ6jIW3" crossorigin="anonymous">
    <link href="https://fonts.cdnfonts.com/css/metropolis-2" rel="stylesheet" crossorigin="anonymous">
    <link rel="stylesheet" href="/ux/main.css" onload="document.body.style.visibility = 'initial'">
    <link rel="icon" type="image/svg+xml" href="/favicon.svg">
`

    document.documentElement.lang = 'en';

    // how Bootstrap & DataTables expect dark mode declared.
    document.documentElement.classList.add('dark');

    this.messsage = this.getCookieMessage();
  }

  render() {
    return html`
      <div>
        <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.1.3/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-1BmE4kWBq78iYhFldvKuhfTAU6auU8tT94WrHftjDbrCEXSU1oBoqyl2QvZ6jIW3" crossorigin="anonymous">
        <nav class="navbar navbar-expand-lg navbar-expand-sm navbar-dark bg-dark">
        <div class="container-fluid">
          <a class="navbar-brand" href="#">
            <img src="/favicon.svg" width="30" height="30" class="d-inline-block align-top" alt="">
            Curio Cluster <span class="alert alert-warning">Beta</span>
          </a>
          <button class="navbar-toggler" type="button"  data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation">
            <span class="navbar-toggler-icon"></span>
          </button>
          <div class="collapse navbar-collapse" id="navbarSupportedContent">
            <ul class="navbar-nav me-auto mb-2 mb-lg-0">
              <li class="nav-item">
                <a class="nav-link" href="/">Home</a>
              </li>
              <li class="nav-item">
                <a class="nav-link" href="/config/">Configurations</a>
              </li>
              <li class="nav-item">
                <a class="nav-link" href="/sector/">Sectors</a>
              </li>
              <li class="nav-item">
                <a class="nav-link" href="/pipeline_porep.html">PoRep</a>
              </li>
              <li>
                <hr class="dropdown-divider">
              </li>
              <li class="nav-item">
                <a class="nav-link" href="https://lotus.filecoin.io/storage-providers/curio/overview/" target="_blank">Documentation</a>
              </li>
            </ul>
          </div>
          </div>
        </nav>
        ${this.message? html`<div class="alert alert-primary" role="alert">${this.message}</div>`: html``}
        <slot class="curio-slot"></slot>
      </div>

    `;
  }
  
  getCookieMessage() {
    const name = 'message';
    const cookies = document.cookie.split(';');
    for (let i = 0; i < cookies.length; i++) {
      const cookie = cookies[i].trim();
      if (cookie.startsWith(name + '=')) {
        var val = cookie.substring(name.length + 1);
        document.cookie = name + '=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;';
        return val;
      }
    }
    return null;
  }

};

customElements.define('curio-ux', CurioUX);