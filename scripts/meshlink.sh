#!/usr/bin/env bash
# MeshLink CLI Tool (Linux/macOS)

OS=$(uname -s)
COMMAND=$1
ADDR_FILE="/etc/meshlink/data/address.txt"
STATE_FILE="/etc/meshlink/data/state.json"

case "$COMMAND" in
    stats)
        echo -e "\033[1;34m=== MeshLink 节点状态报告 ===\033[0m"
        if pgrep -x "p2p-node" > /dev/null; then
            echo -e "核心状态: \033[0;32m正在运行\033[0m"
        else
            echo -e "核心状态: \033[0;31m已停止\033[0m"
        fi

        if [ -f "$STATE_FILE" ]; then
            # 使用 python 简单解析 JSON 并排版（如果系统有 python）
            # 如果没有，退化到 grep/sed
            echo ""
            echo -e "\033[1;36m[ 本机信息 ]\033[0m"
            python3 -c "import json; s=json.load(open('$STATE_FILE')); print(f'  虚拟 IP  : {s[\"self_vip\"]}'); print(f'  节点 ID  : {s[\"self_id\"]}')" 2>/dev/null || {
                grep "SelfVIP" "$STATE_FILE" | cut -d'"' -f4
            }

            echo ""
            echo -e "\033[1;33m[ 已连接的对等节点 ]\033[0m"
            echo -e "  \033[1m虚拟 IP        连接方式    节点 ID\033[0m"
            python3 -c "import json; s=json.load(open('$STATE_FILE')); [print(f'  {v:<15} {\"直连\" if p[\"direct\"] else \"中继\":<8} {p[\"id\"]}') for v,p in s[\"peers\"].items()]" 2>/dev/null || echo "  (正在获取列表...)"
            
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
