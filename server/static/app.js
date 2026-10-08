// AI 学习助手前端应用
class AIAssistant {
    constructor() {
        this.currentMode = 'chat';
        this.history = [];
        this.isLoading = false;
        
        this.init();
    }

    init() {
        // 绑定导航事件
        document.querySelectorAll('.nav-item').forEach(item => {
            item.addEventListener('click', () => this.switchMode(item.dataset.mode));
        });

        // 绑定发送事件
        document.getElementById('send-btn').addEventListener('click', () => this.sendMessage());
        document.getElementById('user-input').addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.sendMessage();
            }
        });

        // 自动调整输入框高度
        document.getElementById('user-input').addEventListener('input', (e) => {
            e.target.style.height = 'auto';
            e.target.style.height = Math.min(e.target.scrollHeight, 120) + 'px';
        });

        // 知识库操作
        document.getElementById('add-knowledge')?.addEventListener('click', () => this.addKnowledge());
        document.getElementById('clear-knowledge')?.addEventListener('click', () => this.clearKnowledge());
        document.getElementById('knowledge-input')?.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') this.addKnowledge();
        });
    }

    switchMode(mode) {
        if (this.isLoading) return;
        this.currentMode = mode;
        this.history = [];
        
        // 更新导航状态
        document.querySelectorAll('.nav-item').forEach(item => {
            item.classList.toggle('active', item.dataset.mode === mode);
        });

        // 更新标题
        const titles = {
            chat: { title: '💬 智能对话', desc: '自由问答，支持多轮对话' },
            knowledge: { title: '📚 知识库问答', desc: 'RAG 检索增强，基于知识库回答' },
            tools: { title: '🔧 学习工具', desc: '计算器、天气查询、时间获取' },
            code: { title: '💻 代码助手', desc: '代码解释、生成、调试' },
            translate: { title: '🌐 翻译助手', desc: '中英文智能互译' }
        };

        const info = titles[mode] || titles.chat;
        document.getElementById('mode-title').textContent = info.title;
        document.getElementById('mode-desc').textContent = info.desc;

        // 显示/隐藏知识库面板
        const knowledgePanel = document.getElementById('knowledge-panel');
        knowledgePanel.classList.toggle('hidden', mode !== 'knowledge');
        if (mode === 'knowledge') this.refreshKnowledgeCount();

        // 更新输入框提示
        const placeholders = {
            chat: '输入你的问题...',
            knowledge: '基于知识库提问...',
            tools: '例如：计算 123*456 / 北京天气 / 现在几点',
            code: '例如：用Go写快速排序 / 解释这段代码',
            translate: '输入要翻译的文本...'
        };
        document.getElementById('user-input').placeholder = placeholders[mode] || placeholders.chat;

        // 清空对话
        this.clearChat();
    }

    clearChat() {
        const container = document.getElementById('chat-container');
        container.innerHTML = `
            <div class="welcome-message">
                <div class="welcome-icon">👋</div>
                <h2>开始对话</h2>
                <p>在下方输入框中输入你的问题</p>
            </div>
        `;
    }

    async sendMessage() {
        const input = document.getElementById('user-input');
        const message = input.value.trim();
        
        if (!message || this.isLoading) return;

        // 清空输入框
        input.value = '';
        input.style.height = 'auto';

        // 移除欢迎消息
        const welcome = document.querySelector('.welcome-message');
        if (welcome) welcome.remove();

        // 显示用户消息
        this.addMessage('user', message);

        // 显示加载状态
        this.isLoading = true;
        const loadingId = this.addLoadingMessage();

        try {
            let response;
            const streaming = ['chat', 'code', 'translate'].includes(this.currentMode);
            
            if (streaming) {
                // 使用流式响应
                response = await this.streamChat(message);
            } else if (this.currentMode === 'knowledge') {
                response = await this.queryKnowledge(message);
            } else if (this.currentMode === 'tools') {
                response = await this.useTools(message);
            }

            // 移除加载状态
            this.removeLoadingMessage(loadingId);

            // 如果不是流式响应，显示结果
            if (response && !streaming) {
                this.addMessage('assistant', response);
            }

            // 更新历史
            this.history.push({ role: 'user', content: message });
            if (response) {
                this.history.push({ role: 'assistant', content: response });
            }

            // 限制历史长度
            if (this.history.length > 10) {
                this.history = this.history.slice(-10);
            }

        } catch (error) {
            this.removeLoadingMessage(loadingId);
            this.addMessage('assistant', `❌ 错误: ${error.message}`);
        }

        this.isLoading = false;
    }

    async streamChat(message) {
        const response = await fetch('/api/chat/stream', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                message,
                history: this.history,
                mode: this.currentMode
            })
        });

        if (!response.ok) {
            throw new Error('请求失败');
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let fullContent = '';
        let messageElement = null;

        let buffer = '';
        let completed = false;
        try {
            while (!completed) {
                const { done, value } = await reader.read();
                buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
                // A JSON event (or Chinese character) may span multiple network chunks.
                let newline;
                while ((newline = buffer.indexOf('\n')) !== -1) {
                    const line = buffer.slice(0, newline).trimEnd();
                    buffer = buffer.slice(newline + 1);
                    if (!line.startsWith('data:')) continue;
                    const data = JSON.parse(line.slice(5).trimStart());
                    if (data.error) throw new Error(data.error);
                    if (data.content) {
                        fullContent += data.content;
                        if (!messageElement) {
                            document.querySelector('.typing-indicator')?.closest('.message')?.remove();
                            messageElement = this.addMessage('assistant', '', true);
                        }
                        messageElement.querySelector('.message-content').innerHTML = this.formatMessage(fullContent);
                        this.scrollToBottom();
                    }
                    if (data.done) {
                        completed = true;
                        break;
                    }
                }
                if (done) break;
            }
            if (!completed) throw new Error('连接中断，回复未完成，请重试');
            return fullContent;
        } finally {
            try { await reader.cancel(); } finally { reader.releaseLock(); }
        }
    }

    async refreshKnowledgeCount() {
        const count = document.getElementById('knowledge-count');
        count.textContent = '加载中...';
        try {
            const response = await fetch('/api/knowledge/count');
            if (!response.ok) throw new Error('读取知识数量失败');
            const data = await response.json();
            if (data.error) throw new Error(data.error);
            count.textContent = `${data.count} 条记录`;
        } catch (error) {
            count.textContent = '数量读取失败';
        }
    }

    async queryKnowledge(message) {
        const response = await fetch('/api/knowledge/query', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ message })
        });

        const data = await response.json();
        if (data.error) throw new Error(data.error);
        return data.content;
    }

    async useTools(query) {
        const response = await fetch('/api/tools', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ query })
        });

        const data = await response.json();
        if (data.error) throw new Error(data.error);
        return data.content;
    }

    async addKnowledge() {
        const input = document.getElementById('knowledge-input');
        const content = input.value.trim();
        
        if (!content) return;

        try {
            const response = await fetch('/api/knowledge/add', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ content })
            });

            const data = await response.json();
            if (data.success) {
                input.value = '';
                if (data.warning) {
                    await this.refreshKnowledgeCount();
                    this.addMessage('assistant', `✅ 知识已添加。${data.warning}`);
                } else {
                    document.getElementById('knowledge-count').textContent = `${data.count} 条记录`;
                    this.addMessage('assistant', `✅ 知识已添加，当前共 ${data.count} 条记录`);
                }
            } else {
                throw new Error(data.error);
            }
        } catch (error) {
            this.addMessage('assistant', `❌ 添加失败: ${error.message}`);
        }
    }

    async clearKnowledge() {
        if (!confirm('确定要清空知识库吗？')) return;

        try {
            const response = await fetch('/api/knowledge/clear', {
                method: 'POST'
            });

            const data = await response.json();
            if (data.success) {
                document.getElementById('knowledge-count').textContent = '0 条记录';
                this.addMessage('assistant', '✅ 知识库已清空');
            } else {
                throw new Error(data.error || '清空失败');
            }
        } catch (error) {
            this.addMessage('assistant', `❌ 清空失败: ${error.message}`);
        }
    }

    addMessage(role, content, returnElement = false) {
        const container = document.getElementById('chat-container');
        
        const messageDiv = document.createElement('div');
        messageDiv.className = `message ${role}`;
        
        const avatar = role === 'user' ? '👤' : '🤖';
        
        messageDiv.innerHTML = `
            <div class="message-avatar">${avatar}</div>
            <div class="message-content">${this.formatMessage(content)}</div>
        `;
        
        container.appendChild(messageDiv);
        this.scrollToBottom();

        if (returnElement) return messageDiv;
    }

    addLoadingMessage() {
        const container = document.getElementById('chat-container');
        const id = 'loading-' + Date.now();
        
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message assistant';
        messageDiv.id = id;
        messageDiv.innerHTML = `
            <div class="message-avatar">🤖</div>
            <div class="message-content">
                <div class="typing-indicator">
                    <span></span>
                    <span></span>
                    <span></span>
                </div>
            </div>
        `;
        
        container.appendChild(messageDiv);
        this.scrollToBottom();
        
        return id;
    }

    removeLoadingMessage(id) {
        const element = document.getElementById(id);
        if (element) element.remove();
    }

    formatMessage(content) {
        if (!content) return '';
        
        // 转义 HTML
        let formatted = content
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');

        // 处理代码块
        formatted = formatted.replace(/```(\w*)\n([\s\S]*?)```/g, (match, lang, code) => {
            return `<pre><code class="language-${lang}">${code.trim()}</code></pre>`;
        });

        // 处理行内代码
        formatted = formatted.replace(/`([^`]+)`/g, '<code>$1</code>');

        // 处理换行
        formatted = formatted.replace(/\n/g, '<br>');

        return formatted;
    }

    scrollToBottom() {
        const container = document.getElementById('chat-container');
        container.scrollTop = container.scrollHeight;
    }
}

// 初始化应用
document.addEventListener('DOMContentLoaded', () => {
    window.app = new AIAssistant();
});
