import { LitElement, html, css } from 'https://cdn.jsdelivr.net/gh/lit/dist@3/all/lit-all.min.js';
window.customElements.define('chain-connectivity', class MyElement extends LitElement {
    constructor() {
        super();
        this.data = [];
        this.loadData();
    }
    loadData() {
        const eventSource = new EventSource('/api/debug/chain-state-sse');
        eventSource.onmessage = (event) => {
            this.data = JSON.parse(event.data);
            super.requestUpdate();
        };
        eventSource.onerror = (error) => {
            console.error('Error:', error);
            loadData();
        };
    };

    static get styles() {
        return [css`
        :host {
            box-sizing: border-box; /* Don't forgert this to include padding/border inside width calculation */
        }
        .success {
            color: green;
        }
        .warning {
            color: yellow;
        }
        .error {
            color: red;
        }
    `];
    }
    render = () => html`
  <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.1.3/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-1BmE4kWBq78iYhFldvKuhfTAU6auU8tT94WrHftjDbrCEXSU1oBoqyl2QvZ6jIW3" crossorigin="anonymous">
    <link rel="stylesheet" href="/ux/main.css">
  <table class="table table-dark">
    <thead>
        <tr>
            <th>RPC Address</th>
            <th>Reachability</th>
            <th>Sync Status</th>
            <th>Version</th>
        </tr>
    </thead>
    <tbody>
        ${this.data.map(item => html`
        <tr>
            <td>${item.Address}</td>
            <td>${item.Reachable ? html`<span class="alert alert-success">ok</span>` : html`<span class="alert altert-danger">FAIL</span>`}</td>
            <td>${item.SyncState === "ok" ? html`<span class="alert alert-success">ok</span>` : html`<span class="alert alert-warning">${item.SyncState}</span>`}</td>
            <td>${item.Version}</td>
        </tr>
        `)}
    </tbody>
  </table>`
});
