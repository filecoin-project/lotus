class JsonRpcClient {
    static instance = null;

    static async getInstance() {
        if (!JsonRpcClient.instance) {
            JsonRpcClient.instance = (async () => {
                const client = new JsonRpcClient('/api/webrpc/v0');
                await client.connect();
                return client;
            })();
        }
        return await JsonRpcClient.instance;
    }


    constructor(url) {
        if (JsonRpcClient.instance) {
            throw new Error("Error: Instantiation failed: Use getInstance() instead of new.");
        }
        this.url = url;
        this.requestId = 0;
        this.pendingRequests = new Map();
    }

    async connect() {
        return new Promise((resolve, reject) => {
            this.ws = new WebSocket(this.url);

            this.ws.onopen = () => {
                console.log("Connected to the server");
                resolve();
            };

            this.ws.onclose = () => {
                console.log("Connection closed, attempting to reconnect...");
                setTimeout(() => this.connect().then(resolve, reject), 1000);  // Reconnect after 1 second
            };

            this.ws.onerror = (error) => {
                console.error("WebSocket error:", error);
                reject(error);
            };

            this.ws.onmessage = (message) => {
                this.handleMessage(message);
            };
        });
    }

    handleMessage(message) {
        const response = JSON.parse(message.data);
        const { id, result, error } = response;

        const resolver = this.pendingRequests.get(id);
        if (resolver) {
            if (error) {
                resolver.reject(error);
            } else {
                resolver.resolve(result);
            }
            this.pendingRequests.delete(id);
        }
    }

    call(method, params = []) {
        const id = ++this.requestId;
        const request = {
            jsonrpc: "2.0",
            method: "CurioWeb." + method,
            params,
            id,
        };

        return new Promise((resolve, reject) => {
            this.pendingRequests.set(id, { resolve, reject });

            if (this.ws.readyState === WebSocket.OPEN) {
                this.ws.send(JSON.stringify(request));
            } else {
                reject('WebSocket is not open');
            }
        });
    }
}

async function init() {
    const client = await JsonRpcClient.getInstance();
    console.log("webrpc backend:", await client.call('Version', []))
}

init();

export default async function(method, params = []) {
    const i = await JsonRpcClient.getInstance();
    return await i.call(method, params);
}
