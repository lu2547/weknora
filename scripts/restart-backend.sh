#!/bin/bash
# WeKnora app (backend + frontend) restart script
#
# Usage:
#   ./scripts/restart-backend.sh          # Rebuild and restart app (backend + frontend)
#   ./scripts/restart-backend.sh --infra  # Also restart infra containers via restart-infra.sh
#   ./scripts/restart-backend.sh --stop   # Kill backend & frontend processes only
#
# Log output:
#   Backend  -> /tmp/weknora-app.log     (live tail: make dev-logs-app)
#   Frontend -> /tmp/weknora-frontend.log

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
LOG_FILE="/tmp/weknora-app.log"
PID_FILE="/tmp/weknora-app.pid"

RESTART_INFRA=false
STOP_ONLY=false
for arg in "$@"; do
  case "$arg" in
    --infra) RESTART_INFRA=true ;;
    --stop)  STOP_ONLY=true ;;
    *) log_warn "Unknown argument: $arg" ;;
  esac
done

cd "$PROJECT_ROOT"

# ---- check docker ----
check_docker() {
  if ! docker info >/dev/null 2>&1; then
    log_error "Docker is not running. Please start Docker Desktop first."
    exit 1
  fi
  log_success "Docker is running"
}

# ---- kill old backend process ----
stop_app() {
  log_info "Stopping old backend process..."

  if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
      kill "$OLD_PID" 2>/dev/null && log_success "Killed PID=$OLD_PID"
    fi
    rm -f "$PID_FILE"
  fi

  # kill any process on port 28080
  PIDS=$(lsof -ti:28080 2>/dev/null || true)
  if [ -n "$PIDS" ]; then
    echo "$PIDS" | xargs kill -9 2>/dev/null || true
    log_success "Released port 28080"
  fi

  pkill -f "go run.*cmd/server/main.go" 2>/dev/null || true
  pkill -f "cmd/server/main.go" 2>/dev/null || true
  sleep 1
}

# ---- restart infra containers (delegates to restart-infra.sh) ----
restart_infra() {
  bash "$SCRIPT_DIR/restart-infra.sh"
}

# ---- wait for postgres ----
wait_for_postgres() {
  log_info "Waiting for PostgreSQL to be ready..."
  MAX=30
  COUNT=0
  until docker exec WeKnora-postgres-dev pg_isready -U weknora >/dev/null 2>&1; do
    COUNT=$((COUNT + 1))
    if [ "$COUNT" -ge "$MAX" ]; then
      log_error "PostgreSQL not ready after ${MAX} retries"
      log_error "Check logs: docker logs WeKnora-postgres-dev"
      exit 1
    fi
    printf "."
    sleep 2
  done
  echo ""
  log_success "PostgreSQL is ready"
}

# ---- load env ----
load_env() {
  if [ ! -f ".env" ]; then
    log_error ".env file not found. Copy from .env.example first."
    exit 1
  fi

  set -a
  # shellcheck disable=SC1091
  source .env
  set +a

  # Override Docker-internal addresses with localhost for local dev
  export DB_HOST=localhost
  export REDIS_ADDR="localhost:${REDIS_PORT:-6379}"
  export MILVUS_ADDRESS="localhost:${MILVUS_PORT:-19530}"
  export DOCREADER_ADDR="localhost:${DOCREADER_PORT:-50051}"
  export DOCREADER_TRANSPORT=grpc
  export MINIO_ENDPOINT="localhost:${MINIO_PORT:-9000}"
  export NEO4J_URI="bolt://localhost:7687"
  export QDRANT_HOST=localhost
  export OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317"

  log_success "Env loaded (DB=${DB_HOST}:${DB_PORT:-5432}, Milvus=${MILVUS_ADDRESS})"
}

# ---- run db migration ----
run_migration() {
  log_info "Running database migrations..."
  # ensure migrate binary is in PATH
  export PATH="$PATH:$(go env GOPATH)/bin"
  if ! command -v migrate >/dev/null 2>&1; then
    log_info "Installing golang-migrate..."
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
  fi
  DB_HOST=localhost PATH="$PATH:$(go env GOPATH)/bin" bash "$PROJECT_ROOT/scripts/migrate.sh" up
  log_success "Migrations applied"
}

# ---- build and start backend ----
build_and_start_app() {
  log_info "Checking build (go build ./...)..."

  export CGO_CFLAGS="-Wno-deprecated-declarations -Wno-gnu-folding-constant"
  if [[ "$(uname)" == "Darwin" ]]; then
    export CGO_LDFLAGS="-Wl,-no_warn_duplicate_libraries"
  fi

  BUILD_OUT=$(go build ./... 2>&1 | grep -v "warning:\|note:" || true)
  if echo "$BUILD_OUT" | grep -q "^.*:.*error:"; then
    log_error "Build failed:"
    echo "$BUILD_OUT"
    exit 1
  fi
  log_success "Build OK"

  LDFLAGS="$(./scripts/get_version.sh ldflags) -X 'google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn'"

  log_info "Starting backend (log -> $LOG_FILE)..."
  nohup go run -ldflags="$LDFLAGS" cmd/server/main.go > "$LOG_FILE" 2>&1 &
  APP_PID=$!
  echo "$APP_PID" > "$PID_FILE"
  log_success "Backend started, PID=$APP_PID"
}

# ---- wait for backend ready ----
wait_for_app() {
  log_info "Waiting for backend :28080..."
  MAX=60
  COUNT=0
  while true; do
    if curl -sf http://localhost:28080/health >/dev/null 2>&1; then
      break
    fi
    if grep -q "Listening and serving\|Server is running" "$LOG_FILE" 2>/dev/null; then
      break
    fi
    COUNT=$((COUNT + 1))
    if [ "$COUNT" -ge "$MAX" ]; then
      log_warn "Backend not ready after ${MAX} retries, check log: tail -50 $LOG_FILE"
      return
    fi
    if ! kill -0 "$APP_PID" 2>/dev/null; then
      log_error "Backend process exited unexpectedly. Check log: tail -50 $LOG_FILE"
      exit 1
    fi
    printf "."
    sleep 2
  done
  echo ""
  log_success "Backend ready: http://localhost:28080"
}

# ---- kill old frontend process ----
stop_frontend() {
  pkill -f "vite" 2>/dev/null || true
  pkill -f "npm run dev" 2>/dev/null || true
  FPIDS=$(lsof -ti:25173 2>/dev/null || true)
  if [ -n "$FPIDS" ]; then
    echo "$FPIDS" | xargs kill -9 2>/dev/null || true
  fi
}

# ---- start frontend ----
start_frontend() {
  FRONTEND_DIR="$PROJECT_ROOT/frontend"
  FRONTEND_LOG="/tmp/weknora-frontend.log"
  if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
    log_info "Installing frontend dependencies..."
    npm --prefix "$FRONTEND_DIR" install
  fi
  stop_frontend
  log_info "Starting frontend (log -> $FRONTEND_LOG)..."
  nohup npm --prefix "$FRONTEND_DIR" run dev -- --force > "$FRONTEND_LOG" 2>&1 &
  FPID=$!
  sleep 3
  if kill -0 "$FPID" 2>/dev/null; then
    log_success "Frontend started: http://localhost:25173 (PID=$FPID)"
  else
    log_warn "Frontend may have failed, check: tail -20 $FRONTEND_LOG"
  fi
}

# ---- main ----
log_info "=== WeKnora Restart ==================================="

stop_app

if [ "$STOP_ONLY" = true ]; then
  stop_frontend
  log_success "All processes stopped. Exiting."
  exit 0
fi

check_docker

if [ "$RESTART_INFRA" = true ]; then
  restart_infra
else
  log_info "Checking infra containers..."
  if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q "WeKnora-postgres-dev"; then
    log_warn "postgres container not running, starting infra..."
    docker compose -f docker-compose.dev.yml --profile milvus up -d
  else
    log_success "Infra containers running"
  fi
fi

wait_for_postgres
load_env
run_migration
build_and_start_app
wait_for_app
start_frontend

echo ""
log_success "=== Done ==============================================" 
log_info "  Frontend  : http://localhost:25173"
log_info "  API       : http://localhost:28080"
log_info "  Backend log : tail -f $LOG_FILE"
log_info "  Frontend log: tail -f /tmp/weknora-frontend.log"
log_info "  Stop all  : $0 --stop"
log_info "  Full reset: $0 --infra"
echo ""
