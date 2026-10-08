const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const vm = require('node:vm');

function fixture(events) {
    const input = { value: '你好', style: {} };
    const document = {
        addEventListener() {},
        querySelector() { return null; },
        getElementById() { return input; },
    };
    const bytes = new TextEncoder().encode(events);
    const sandbox = vm.createContext({
        document, TextDecoder,
        fetch: async () => ({ ok: true, body: new ReadableStream({
            start(controller) {
                // Split every byte, including UTF-8 characters and SSE JSON.
                for (const byte of bytes) controller.enqueue(Uint8Array.of(byte));
                controller.close();
            },
        }) }),
    });
    const source = readFileSync(require('node:path').join(__dirname, '../static/app.js'), 'utf8');
    vm.runInContext(source + '\nglobalThis.Assistant = AIAssistant;', sandbox);
    const app = Object.create(sandbox.Assistant.prototype);
    Object.assign(app, { currentMode: 'chat', history: [], isLoading: false });
    const content = { innerHTML: '' };
    const displayed = [];
    app.addMessage = (role, text) => {
        displayed.push({ role, text });
        return { querySelector: () => content };
    };
    app.scrollToBottom = () => {};
    app.addLoadingMessage = () => 'loading';
    app.removeLoadingMessage = () => {};
    return { app, content, displayed };
}

test('stream survives byte splits and history records each turn only once', async () => {
    const { app, content, displayed } = fixture('data: {"content":"你好🌏"}\n\ndata: {"done":true}\n\n');
    await app.sendMessage();
    assert.equal(content.innerHTML, '你好🌏');
    assert.equal(app.history.length, 2);
    assert.equal(app.history[0].role, 'user');
    assert.equal(app.history[1].content, '你好🌏');
    assert.equal(displayed.filter(m => m.role === 'assistant').length, 1);
    assert.equal(app.isLoading, false);
});

test('backend SSE error is shown rather than silently ignored', async () => {
    const { app, displayed } = fixture('data: {"error":"模型不可用"}\n\n');
    await app.sendMessage();
    assert.match(displayed.at(-1).text, /模型不可用/);
    assert.equal(app.history.length, 0);
    assert.equal(app.isLoading, false);
});

test('premature EOF is reported as an incomplete response', async () => {
    const { app } = fixture('data: {"content":"部分回复"}\n\n');
    await assert.rejects(app.streamChat('你好'), /回复未完成/);
});
