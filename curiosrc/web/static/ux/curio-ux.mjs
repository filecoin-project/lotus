import {LitElement, css, html} from 'https://cdn.jsdelivr.net/gh/lit/dist@3/all/lit-all.min.js';


class CurioUX extends LitElement {
  static styles = css`
  .curio-slot {
  }
  `;
  connectedCallback() { 
    super.connectedCallback(); 
    const links = [
      "https://unpkg.com/@cds/core/global.min.css",
      "https://unpkg.com/@cds/city/css/bundles/default.min.css",
      "https://unpkg.com/@cds/core/styles/theme.dark.min.css",
      "https://unpkg.com/@clr/ui/clr-ui.min.css",
      "https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css"
    ];

    const head = document.head;
    links.forEach(link => {
      const linkNode = document.createElement('link');
      linkNode.rel = 'stylesheet';
      linkNode.href = link;
      head.appendChild(linkNode);
    });

    var theme = document.createAttribute('cds-theme');
    theme.value =  localStorage.getItem('theme') || 'dark';
    document.body.attributes.setNamedItem(theme);

    var cdsText = document.createAttribute('cds-text');
    cdsText.value = 'body';
    document.body.attributes.setNamedItem(cdsText);

    document.body.style.visibility = 'initial';
  }

  render() {
    return html`
      <!-- wrap the slot -->
      <div >
        <slot class="curio-slot"></slot>
      </div>

    `;
  }
  
}

customElements.define('curio-ux', CurioUX);