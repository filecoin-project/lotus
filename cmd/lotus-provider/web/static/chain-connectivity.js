import { LitElement, html, css } from 'https://cdn.jsdelivr.net/npm/lit-html@3.1.0/lit-html.min.js';
window.customElements.define('chain-connectivity', class MyElement extends LitElement {
    constructor() {
        super();
        this.data = [];
        this.loadData();
    }
    loadData() {
        const eventSource = new EventSource('/api/debug/chain-state-sse');
        eventSource.onmessage = (event) => {
            this.data.push(JSON.parse(event.data));
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
        table {
            border-collapse: collapse;
        }

        table td, table th {
            border-left: 1px solid #f0f0f0;
            padding: 1px 5px;
        }

        table tr td:first-child, table tr th:first-child {
            border-left: none;
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
  <table>
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
            <td>{{.Address}}</td>
            <td>${item.Address}</td>
            <td>${item.Reachable ? html`<span class="success">ok</span>` : html`<span class="error">FAIL</span>`}</td>
            <td>${item.SyncState === "ok" ? html`<span class="success">ok</span>` : html`<span class="warning">${item.SyncState}</span>`}</td>
            <td>${item.Version}</td>
        </tr>
        `)}
        <tr>
            <td colspan="4">Data incoming...</td>
        </tr>
    </tbody>
  </table>`
});
