import { html, css, LitElement } from 'lit';

class ClarityComponent extends LitElement {
  static styles = css`
    @import url("https://unpkg.com/@cds/core/global.min.css");
    @import url("https://unpkg.com/@cds/core/styles/theme.dark.min.css");
    @import url("https://unpkg.com/@clr/ui/clr-ui.min.css");
  `;
  connectedCallback() { 
    super.connectedCallback(); 
    var theme = document.createAttribute('cds-theme');
    theme.value = 'dark';
    // Check local settings for theme value
    var localTheme = localStorage.getItem('theme');
    if (localTheme !== '') {
      theme.value = localTheme;
    }
    document.body.attributes.setNamedItem(theme);
  }
  render() {
    return html`
      <!-- Your HTML template here -->
    `;
  }
  
}

customElements.define('curio-wrap', ClarityComponent);