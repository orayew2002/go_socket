#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# run.sh – Docker lifecycle helper for the SMS Service
#
# Usage:
#   ./run.sh build      Build the Docker image
#   ./run.sh run        Start the container (detached)
#   ./run.sh stop       Stop and remove the container
#   ./run.sh restart    Restart the running container
#   ./run.sh logs       Tail container logs
#   ./run.sh status     Show container status
#   ./run.sh rebuild    Stop → build → run  (full redeploy)
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
IMAGE_NAME="sms_service"
CONTAINER_NAME="sms_service"
ENV_FILE=".env"

# Colour helpers
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Colour

info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# ── Guard: must be run from the project root ──────────────────────────────────
[ -f "Dockerfile" ] || error "Dockerfile not found. Run this script from the project root."

# ── Commands ──────────────────────────────────────────────────────────────────
cmd_build() {
    info "Building Docker image '${IMAGE_NAME}'..."
    docker build -t "${IMAGE_NAME}" .
    success "Image '${IMAGE_NAME}' built successfully."
}

cmd_run() {
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        warn "Container '${CONTAINER_NAME}' already exists. Run './run.sh restart' or './run.sh rebuild'."
        exit 1
    fi

    [ -f "${ENV_FILE}" ] || error ".env file not found. Create one before starting."

    info "Starting container '${CONTAINER_NAME}'..."
    docker run -d \
        --name "${CONTAINER_NAME}" \
        --network host \
        --env-file "${ENV_FILE}" \
        --restart unless-stopped \
        "${IMAGE_NAME}"

    success "Container '${CONTAINER_NAME}' is running."
    info "Logs: ./run.sh logs"
}

cmd_stop() {
    if ! docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        warn "Container '${CONTAINER_NAME}' does not exist."
        exit 0
    fi

    info "Stopping container '${CONTAINER_NAME}'..."
    docker stop "${CONTAINER_NAME}"
    docker rm   "${CONTAINER_NAME}"
    success "Container '${CONTAINER_NAME}' stopped and removed."
}

cmd_restart() {
    if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        warn "Container '${CONTAINER_NAME}' is not running. Starting it..."
        cmd_run
        return
    fi

    info "Restarting container '${CONTAINER_NAME}'..."
    docker restart "${CONTAINER_NAME}"
    success "Container '${CONTAINER_NAME}' restarted."
}

cmd_logs() {
    docker logs -f "${CONTAINER_NAME}"
}

cmd_status() {
    echo ""
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        success "Container '${CONTAINER_NAME}' is RUNNING."
        docker ps --filter "name=^${CONTAINER_NAME}$" \
            --format "  ID: {{.ID}}\n  Image: {{.Image}}\n  Status: {{.Status}}\n  Ports: {{.Ports}}"
    elif docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        warn "Container '${CONTAINER_NAME}' EXISTS but is STOPPED."
    else
        warn "Container '${CONTAINER_NAME}' does NOT exist."
    fi
    echo ""
}

cmd_rebuild() {
    info "Starting full redeploy..."
    cmd_stop  || true   # ignore error if container doesn't exist
    cmd_build
    cmd_run
    success "Redeploy complete."
}

cmd_help() {
    echo ""
    echo "  Usage: ./run.sh <command>"
    echo ""
    echo "  Commands:"
    echo "    build     Build the Docker image from Dockerfile"
    echo "    run       Start the container in detached mode"
    echo "    stop      Stop and remove the container"
    echo "    restart   Restart the running container"
    echo "    logs      Tail container logs (Ctrl+C to exit)"
    echo "    status    Show container running status"
    echo "    rebuild   Full redeploy: stop → build → run"
    echo ""
}

# ── Entry point ───────────────────────────────────────────────────────────────
case "${1:-help}" in
    build)   cmd_build   ;;
    run)     cmd_run     ;;
    stop)    cmd_stop    ;;
    restart) cmd_restart ;;
    logs)    cmd_logs    ;;
    status)  cmd_status  ;;
    rebuild) cmd_rebuild ;;
    *)       cmd_help    ;;
esac
