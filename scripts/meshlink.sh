#!/usr/bin/env bash
# MeshLink Linux CLI Tool

COMMAND=$1

case "$COMMAND" in
    status)
        systemctl status meshlink
        ;;
    start)
        sudo systemctl start meshlink
        echo "MeshLink started."
        ;;
    stop)
        sudo systemctl stop meshlink
        echo "MeshLink stopped."
        ;;
    restart)
        sudo systemctl restart meshlink
        echo "MeshLink restarted."
        ;;
    logs)
        journalctl -u meshlink -f
        ;;
    address)
        if [ -f /etc/meshlink/data/address.txt ]; then
            cat /etc/meshlink/data/address.txt
        else
            echo "Address file not found. Is the service running?"
        fi
        ;;
    *)
        echo "MeshLink CLI Tool"
        echo "Usage: meshlink {start|stop|restart|status|logs|address}"
        echo ""
        echo "  status   : View systemd service status"
        echo "  start    : Start the meshlink service"
        echo "  stop     : Stop the meshlink service"
        echo "  restart  : Restart the meshlink service"
        echo "  logs     : Tail the meshlink service logs (Ctrl+C to exit)"
        echo "  address  : Print the node's Virtual IP and Multiaddr (Peer ID)"
        exit 1
        ;;
esac
