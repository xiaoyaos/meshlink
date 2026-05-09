#!/usr/bin/env bash
# MeshLink CLI 管理工具 (Linux/macOS)

OS=$(uname -s)
COMMAND=$1
ADDR_FILE="/etc/meshlink/data/address.txt"
STATE_FILE="/etc/meshlink/data/state.json"

show_help() {
    echo -e "\033[1;32mMeshLink P2P 虚拟网状网络管理工具\033[0m"
    echo ""
    echo "用法: meshlink <命令> [参数]"
    echo ""
    echo "核心命令:"
    echo "  stats       查看当前节点运行状态、虚拟 IP 及在线成员列表"
    echo "  start       启动 MeshLink 后台服务"
    echo "  stop        停止 MeshLink 后台服务"
    echo "  restart     重启 MeshLink 后台服务"
    echo "  logs        查看实时运行日志 (Ctrl+C 退出)"
    echo "  test <IP>   测试与指定虚拟 IP 的 P2P 链路延迟"
    echo ""
    echo "其他:"
    echo "  -h, --help  显示此帮助信息"
    echo "  address     (已弃用) 请使用 stats 查看地址"
    echo ""
    echo "示例:"
    echo "  meshlink stats"
    echo "  meshlink test 10.1.2.3"
    echo ""
}

if [[ "$COMMAND" == "--help" || "$COMMAND" == "-h" || -z "$COMMAND" ]]; then
    show_help
    exit 0
fi

case "$COMMAND" in
    stats)
        echo -e "\033[1;34m=== MeshLink 节点状态报告 ===\033[0m"

        # 提取版本信息
        VERSION="未知"
        if [ -f "$STATE_FILE" ]; then
            VERSION=$(python3 -c "import json; print(json.load(open('$STATE_FILE'))['version'])" 2>/dev/null)
        elif [ -f "$ADDR_FILE" ]; then
            VERSION=$(grep "版本:" "$ADDR_FILE" | cut -d' ' -f2)
        fi
        echo -e "软件版本: \033[1;37m$VERSION\033[0m"

        if pgrep -x "p2p-node" > /dev/null; then
            echo -e "核心状态: \033[0;32m正在运行\033[0m"
        else
            echo -e "核心状态: \033[0;31m已停止\033[0m"
        fi

        if [ -f "$STATE_FILE" ]; then
            # 使用 python 简单解析 JSON 并排版
            echo ""
            echo -e "\033[1;36m[ 本机信息 ]\033[0m"
            python3 -c "import json; s=json.load(open('$STATE_FILE')); print(f'  节点类型 : {s.get(\"node_type\", \"未知\")}'); print(f'  虚拟 IP  : {s[\"self_vip\"]}'); print(f'  节点 ID  : {s[\"self_id\"]}')" 2>/dev/null || {
                grep -E "SelfVIP|SelfID" "$STATE_FILE"
            }

            echo ""
            echo -e "\033[1;33m[ 已连接的对等节点 ]\033[0m"
            echo -e "  \033[1m虚拟 IP        连接方式    节点 ID\033[0m"
            python3 -c "import json; s=json.load(open('$STATE_FILE')); [print(f'  {v:<15} {\"直连\" if p[\"direct\"] else \"中继\":<8} {p[\"id\"]}') for v,p in s[\"peers\"].items()]" 2>/dev/null || echo "  (暂无在线节点)"
            
            echo ""
            echo -e "\033[1;35m[ 简写地址 (复制给他人使用) ]\033[0m"
            if [ -f "$ADDR_FILE" ]; then
                 sed -n '/简写格式/,/标准/p' "$ADDR_FILE" | grep -E '^[0-9]' | while read line; do
                    echo -e "  🔗 $line"
                done
            fi
        else
            # 退化逻辑：如果 state.json 还没生成，尝试读 address.txt
            if [ -f "$ADDR_FILE" ]; then
                VIP=$(grep "虚拟IP" "$ADDR_FILE" | cut -d' ' -f2)
                echo -e "虚拟 IP : \033[1;32m$VIP\033[0m"
                echo ""
                echo -e "\033[1;33m[ 简写地址 ]\033[0m"
                sed -n '/简写格式/,/标准/p' "$ADDR_FILE" | grep -E '^[0-9]' | while read line; do echo "  🔗 $line"; done
            fi
        fi
        echo ""
        ;;
    test)
        TARGET=$2
        if [[ -z "$TARGET" ]]; then
            echo "用法: meshlink test <目标虚拟IP>"
            exit 1
        fi
        echo -e "\033[1;34m[测试] 正在测试到 $TARGET 的 P2P 链路延迟...\033[0m"
        ping -c 4 "$TARGET"
        ;;
    start)
        if [[ "$OS" == "Darwin" ]]; then
            sudo launchctl load -w /Library/LaunchDaemons/com.meshlink.p2p.plist
        else
            sudo systemctl start meshlink
        fi
        echo "MeshLink 已启动。"
        ;;
    stop)
        if [[ "$OS" == "Darwin" ]]; then
            sudo launchctl unload -w /Library/LaunchDaemons/com.meshlink.p2p.plist
        else
            sudo systemctl stop meshlink
        fi
        echo "MeshLink 已停止。"
        ;;
    restart)
        $0 stop
        sleep 1
        $0 start
        ;;
    logs)
        if [[ "$OS" == "Darwin" ]]; then
            tail -f /var/log/meshlink.log
        else
            journalctl -u meshlink -f
        fi
        ;;
    *)
        echo "MeshLink 管理工具"
        echo "用法: meshlink {stats|start|stop|restart|logs}"
        exit 1
        ;;
esac
