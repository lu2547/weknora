#!/bin/bash
# WeKnora infrastructure containers restart script
# Services: postgres / redis / milvus / docreader
#
# Usage:
#   ./scripts/restart-infra.sh           # Start or restart (down + up)
#   ./scripts/restart-infra.sh --stop    # Stop all infra containers
#   ./scripts/restart-infra.sh --status  # Show container status

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()    { printf "${BLUE}[INFO]${NC}    %s\n" "$1"; }
log_success() { printf "${GREEN}[OK]${NC}      %s\n" "$1"; }
log_warn()    { printf "${YELLOW}[WARN]${NC}    %s\n" "$1"; }
log_error()   { printf "${RED}[ERROR]${NC}   %s\n" "$1"; }

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
COMPOSE_FILE="docker-compose.dev.yml"
# milvus profile pulls in milvus/etcd/minio alongside the always-on services
# (postgres / redis / docreader / jaeger defined at compose root).
COMPOSE_ARGS=(-f "$COMPOSE_FILE" --profile milvus)

ACTION="restart"
for arg in "$@"; do
  case "$arg" in
    --stop)   ACTION="stop" ;;
    --status) ACTION="status" ;;
    --help|-h)
      sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) log_warn "Unknown argument: $arg" ;;
  esac
done

cd "$PROJECT_ROOT"

check_docker() {
  if ! docker info >/dev/null 2>&1; then
    log_error "Docker is not running. Please start Docker Desktop first."
    exit 1
  fi
  log_success "Docker is running"
}

show_status() {
  log_info "Infra containers status:"
  docker ps --filter "name=WeKnora-" \
    --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
}

case "$ACTION" in
  stop)
    log_info "=== Stop WeKnora infra =============================="
    check_docker
    docker compose "${COMPOSE_ARGS[@]}" down --remove-orphans
    log_success "Infra containers stopped"
    ;;
  status)
    check_docker
    show_status
    ;;
  restart)
    log_info "=== Restart WeKnora infra ==========================="
    check_docker
    log_info "Bringing down existing containers..."
    docker compose "${COMPOSE_ARGS[@]}" down --remove-orphans
    sleep 2
    log_info "Starting postgres / redis / milvus / docreader..."
    docker compose "${COMPOSE_ARGS[@]}" up -d
    sleep 2
    show_status
    log_success "=== Done ============================================"
    ;;
esac
