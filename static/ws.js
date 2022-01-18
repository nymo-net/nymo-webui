'use strict';

const loc = window.location;
const ws = new WebSocket(((loc.protocol === "https:") ? "wss://" : "ws://") + loc.host);
const handles = {'err': console.error};

ws.onclose = function (ce) {
    handles.err(ce.reason);
}

ws.onmessage = function ({data}) {
    const [command, d] = JSON.parse(data);
    handles[command]?.(d);
}

export function send(command, data) {
    ws.send(JSON.stringify([command, data]));
}

export function register(command, handle) {
    handles[command] = handle;
}
