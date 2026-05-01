import './style.css';
import { GetInfo, StartVPN, StopVPN, GetPeerCount } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

// DOM Elements
const peerIdEl = document.getElementById('peer-id');
const virtualIpEl = document.getElementById('virtual-ip');
const statusDotEl = document.getElementById('status-dot');
const statusPulseEl = document.getElementById('status-pulse');
const statusTextEl = document.getElementById('status-text');
const peerCountEl = document.getElementById('peer-count');
const bootstrapInput = document.getElementById('bootstrap-input');
const toggleBtn = document.getElementById('toggle-btn');
const btnText = toggleBtn.querySelector('.btn-text');
const logView = document.getElementById('log-view');
const canvas = document.getElementById('network-canvas');

// State
let isRunning = false;
let peerCount = 0;

// ==========================================
// P2P Network Canvas Animation
// ==========================================
const ctx = canvas.getContext('2d');
let nodes = [];
let animationFrameId;

function resizeCanvas() {
    const parent = canvas.parentElement;
    canvas.width = parent.clientWidth;
    canvas.height = parent.clientHeight;
}
window.addEventListener('resize', resizeCanvas);

class Node {
    constructor(x, y, isLocal = false) {
        this.x = x;
        this.y = y;
        this.vx = (Math.random() - 0.5) * 0.5;
        this.vy = (Math.random() - 0.5) * 0.5;
        this.radius = isLocal ? 8 : 4;
        this.isLocal = isLocal;
        this.color = isLocal ? '#38bdf8' : '#94a3b8';
    }

    update() {
        if (!this.isLocal) {
            this.x += this.vx;
            this.y += this.vy;

            // Bounce off walls
            if (this.x < 0 || this.x > canvas.width) this.vx *= -1;
            if (this.y < 0 || this.y > canvas.height) this.vy *= -1;
        } else {
            // Local node stays in center
            this.x = canvas.width / 2;
            this.y = canvas.height / 2;
        }
    }

    draw() {
        ctx.beginPath();
        ctx.arc(this.x, this.y, this.radius, 0, Math.PI * 2);
        ctx.fillStyle = this.color;
        ctx.fill();

        if (this.isLocal && isRunning) {
            // Draw a subtle pulse around local node
            ctx.beginPath();
            ctx.arc(this.x, this.y, this.radius * 3 + Math.sin(Date.now() / 200) * 5, 0, Math.PI * 2);
            ctx.strokeStyle = 'rgba(56, 189, 248, 0.2)';
            ctx.stroke();
        }
    }
}

function initNodes() {
    nodes = [];
    // The central local node
    nodes.push(new Node(canvas.width / 2, canvas.height / 2, true));
    
    // Abstract peers (we generate a visual representation based on peerCount)
    // Add fake floating nodes to make it look cool even if disconnected (optional, but looks better)
    const visualNodesCount = isRunning ? Math.max(peerCount * 3, 5) : 3; 
    
    for (let i = 0; i < visualNodesCount; i++) {
        nodes.push(new Node(Math.random() * canvas.width, Math.random() * canvas.height));
    }
}

function drawNetwork() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    nodes.forEach(node => node.update());

    // Draw connections
    for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
            const dx = nodes[i].x - nodes[j].x;
            const dy = nodes[i].y - nodes[j].y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            
            const maxDist = isRunning ? 150 : 80;

            if (dist < maxDist) {
                ctx.beginPath();
                ctx.moveTo(nodes[i].x, nodes[i].y);
                ctx.lineTo(nodes[j].x, nodes[j].y);
                
                // Opacity based on distance and running state
                let opacity = 1 - (dist / maxDist);
                if (!isRunning) opacity *= 0.2; // Dim when disconnected
                if (nodes[i].isLocal || nodes[j].isLocal) {
                    ctx.strokeStyle = `rgba(56, 189, 248, ${opacity})`;
                } else {
                    ctx.strokeStyle = `rgba(148, 163, 184, ${opacity * 0.5})`;
                }
                
                ctx.stroke();
            }
        }
    }

    nodes.forEach(node => node.draw());
    animationFrameId = requestAnimationFrame(drawNetwork);
}

// ==========================================
// App Logic
// ==========================================

setInterval(async () => {
    if (isRunning) {
        try {
            const count = await GetPeerCount();
            peerCount = count;
            peerCountEl.innerText = `${count} 个节点`;
            
            // Re-adjust nodes array if peer count changes dramatically to make it look dynamic
            if (nodes.length < count * 2 + 1 && count > 0) {
                 nodes.push(new Node(Math.random() * canvas.width, Math.random() * canvas.height));
            }

            if (count > 0) {
                statusDotEl.className = "status-dot online";
                statusPulseEl.className = "status-pulse online";
                statusTextEl.innerText = "已连接";
            } else {
                statusDotEl.className = "status-dot connecting";
                statusPulseEl.className = "status-pulse connecting";
                statusTextEl.innerText = "正在搜索节点...";
            }
        } catch (e) {}
    } else {
        peerCount = 0;
        peerCountEl.innerText = "0 个节点";
    }
}, 3000);

async function init() {
    resizeCanvas();
    initNodes();
    drawNetwork();

    try {
        const [id, ip] = await GetInfo();
        peerIdEl.innerText = id;
        virtualIpEl.innerText = ip;
    } catch (e) {
        appendLog("获取信息失败: " + e, 'error');
    }
}

function appendLog(msg, type = 'normal') {
    const div = document.createElement('div');
    div.className = `log-entry ${type}`;
    
    const time = new Date().toLocaleTimeString([], { hour12: false });
    div.innerText = `[${time}] ${msg}`;
    
    logView.appendChild(div);
    logView.scrollTop = logView.scrollHeight;
}

// Actions
document.getElementById('copy-peer').onclick = () => {
    navigator.clipboard.writeText(peerIdEl.innerText);
    appendLog("节点 ID 已复制到剪贴板", 'system');
};

toggleBtn.onclick = async () => {
    if (!isRunning) {
        toggleBtn.classList.add('disabled');
        btnText.innerText = "正在启动...";
        const err = await StartVPN(bootstrapInput.value);
        if (err) {
            appendLog("严重错误: " + err, 'error');
            toggleBtn.classList.remove('disabled');
            btnText.innerText = "启动网络";
        }
    } else {
        toggleBtn.classList.add('disabled');
        btnText.innerText = "正在停止...";
        await StopVPN();
    }
};

// Events from Go
EventsOn("vpn_log", (msg) => {
    appendLog(msg);
});

EventsOn("vpn_state", (state) => {
    toggleBtn.classList.remove('disabled');
    
    switch (state) {
        case 0: // Disconnected
            isRunning = false;
            statusTextEl.innerText = "未连接";
            statusDotEl.className = "status-dot";
            statusPulseEl.className = "status-pulse";
            
            toggleBtn.classList.remove('stop');
            toggleBtn.classList.add('start');
            btnText.innerText = "启动网络";
            
            initNodes(); // Reset animation
            break;
        case 1: // Connecting
            statusTextEl.innerText = "正在连接...";
            statusDotEl.className = "status-dot connecting";
            statusPulseEl.className = "status-pulse connecting";
            break;
        case 2: // Connected
            isRunning = true;
            statusTextEl.innerText = "已连接";
            statusDotEl.className = "status-dot online";
            statusPulseEl.className = "status-pulse online";
            
            toggleBtn.classList.remove('start');
            toggleBtn.classList.add('stop');
            btnText.innerText = "停止网络";
            
            initNodes(); // Refresh animation nodes
            break;
        case 3: // Error
            statusTextEl.innerText = "错误";
            statusDotEl.className = "status-dot";
            statusPulseEl.className = "status-pulse";
            break;
    }
});

// Start app
init();
