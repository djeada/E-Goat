#!/usr/bin/env bash
set -euo pipefail

PORTS="8080,9000"
APPLY_CHANGES="false"
CHECK_LISTENERS="false"
WAN_CHECK="true"
RESET_CHANGES="false"
PORT_MAP="false"
PORT_MAP_TTL="3600"
STATE_DIR="${HOME:-/tmp}/.config/e-goat"
FIREWALL_STATE_FILE="${STATE_DIR}/setup-network.firewall.state"
PORTMAP_STATE_FILE="${STATE_DIR}/setup-network.portmap.state"

usage() {
  cat <<'EOF'
Usage: ./scripts/setup-network.sh [options]

Options:
  --ports 8080,9000   Comma-separated TCP ports to allow (default: 8080,9000)
  --apply             Apply firewall changes (requires root)
  --reset             Reset previously applied firewall changes (requires root)
  --check-listeners   Verify local listeners are bound on the given ports
  --port-map          Attempt router port-mapping via UPnP or NAT-PMP
  --port-map-ttl SEC  NAT-PMP mapping TTL in seconds (default: 3600)
  --no-wan-check      Skip WAN IP check (avoids external lookup)
  -h, --help          Show this help

Notes:
  - This script only configures the local firewall. Router port-forwarding is
    still required for friends to reach your HTTP/WS servers over the internet.
  - WebRTC without TURN only works when NATs allow direct P2P.
EOF
}

log() { printf '%s\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

is_root() {
  [ "$(id -u)" -eq 0 ]
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

is_private_ipv4() {
  local ip="$1"
  if [[ "$ip" =~ ^10\. ]]; then return 0; fi
  if [[ "$ip" =~ ^192\.168\. ]]; then return 0; fi
  if [[ "$ip" =~ ^172\.([1-2][0-9]|3[0-1])\. ]]; then return 0; fi
  if [[ "$ip" =~ ^100\.(6[4-9]|[7-9][0-9]|1[0-1][0-9]|12[0-7])\. ]]; then return 0; fi
  return 1
}

check_listeners() {
  local ports_csv="$1"
  local ports
  IFS=',' read -r -a ports <<< "$ports_csv"

  if ! has_cmd ss; then
    warn "ss not found; cannot check listeners."
    return
  fi

  log "Checking local listeners..."
  for p in "${ports[@]}"; do
    if ss -lnt | awk '{print $4}' | grep -E "[:.]${p}\$" >/dev/null 2>&1; then
      log "  OK: listening on TCP $p"
    else
      warn "  NOT LISTENING: TCP $p (start the app with make run)"
    fi
  done
}

check_os() {
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    log "OS: ${PRETTY_NAME:-unknown}"
  else
    warn "Cannot detect OS (missing /etc/os-release)"
  fi
}

check_lan_ip() {
  if ! has_cmd ip; then
    warn "ip command not found; skipping LAN IP check."
    return
  fi
  local ips
  ips="$(ip -4 addr show scope global | awk '/inet / {print $2}' | cut -d/ -f1 | tr '\n' ' ')"
  if [ -n "$ips" ]; then
    log "LAN IPv4: $ips"
  else
    warn "No LAN IPv4 found."
  fi
}

check_wan_ip() {
  if [ "$WAN_CHECK" != "true" ]; then
    log "WAN check skipped."
    return
  fi
  if ! has_cmd curl; then
    warn "curl not found; skipping WAN IP check."
    return
  fi
  local wan_ip
  wan_ip="$(curl -fsS https://api.ipify.org || true)"
  if [ -z "$wan_ip" ]; then
    warn "Failed to fetch WAN IP."
    return
  fi
  log "WAN IPv4: $wan_ip"
  if is_private_ipv4 "$wan_ip"; then
    warn "WAN IP is private (likely CGNAT). Direct inbound may not work."
  fi
}

apply_ufw() {
  local ports_csv="$1"
  local ports
  IFS=',' read -r -a ports <<< "$ports_csv"

  log "UFW detected. Applying rules..."
  for p in "${ports[@]}"; do
    ufw allow "${p}/tcp" >/dev/null
  done
  ufw reload >/dev/null
  log "UFW rules applied."
}

reset_ufw() {
  local ports_csv="$1"
  local ports
  IFS=',' read -r -a ports <<< "$ports_csv"

  log "Reverting UFW rules..."
  for p in "${ports[@]}"; do
    ufw delete allow "${p}/tcp" >/dev/null 2>&1 || true
  done
  ufw reload >/dev/null
  log "UFW rules reverted."
}

apply_firewalld() {
  local ports_csv="$1"
  local ports
  IFS=',' read -r -a ports <<< "$ports_csv"

  log "firewalld detected. Applying rules..."
  for p in "${ports[@]}"; do
    firewall-cmd --add-port="${p}/tcp" --permanent >/dev/null
  done
  firewall-cmd --reload >/dev/null
  log "firewalld rules applied."
}

reset_firewalld() {
  local ports_csv="$1"
  local ports
  IFS=',' read -r -a ports <<< "$ports_csv"

  log "Reverting firewalld rules..."
  for p in "${ports[@]}"; do
    firewall-cmd --remove-port="${p}/tcp" --permanent >/dev/null 2>&1 || true
  done
  firewall-cmd --reload >/dev/null
  log "firewalld rules reverted."
}

apply_iptables() {
  local ports_csv="$1"
  local ports
  IFS=',' read -r -a ports <<< "$ports_csv"

  log "Applying iptables rules..."
  for p in "${ports[@]}"; do
    iptables -C INPUT -p tcp --dport "$p" -j ACCEPT 2>/dev/null || \
      iptables -A INPUT -p tcp --dport "$p" -j ACCEPT
  done
  if has_cmd netfilter-persistent; then
    netfilter-persistent save >/dev/null
  fi
  log "iptables rules applied."
}

reset_iptables() {
  local ports_csv="$1"
  local ports
  IFS=',' read -r -a ports <<< "$ports_csv"

  log "Reverting iptables rules..."
  for p in "${ports[@]}"; do
    iptables -D INPUT -p tcp --dport "$p" -j ACCEPT 2>/dev/null || true
  done
  if has_cmd netfilter-persistent; then
    netfilter-persistent save >/dev/null
  fi
  log "iptables rules reverted."
}

write_firewall_state() {
  local tool="$1"
  local ports_csv="$2"

  mkdir -p "$STATE_DIR"
  cat > "$FIREWALL_STATE_FILE" <<EOF
tool=${tool}
ports=${ports_csv}
EOF
}

read_firewall_state() {
  if [ ! -f "$FIREWALL_STATE_FILE" ]; then
    return 1
  fi
  # shellcheck disable=SC1090
  . "$FIREWALL_STATE_FILE"
  if [ -z "${tool:-}" ] || [ -z "${ports:-}" ]; then
    return 1
  fi
  return 0
}

write_portmap_state() {
  local tool="$1"
  local ports_csv="$2"
  local lan_ip="$3"

  mkdir -p "$STATE_DIR"
  cat > "$PORTMAP_STATE_FILE" <<EOF
tool=${tool}
ports=${ports_csv}
lan_ip=${lan_ip}
EOF
}

read_portmap_state() {
  if [ ! -f "$PORTMAP_STATE_FILE" ]; then
    return 1
  fi
  # shellcheck disable=SC1090
  . "$PORTMAP_STATE_FILE"
  if [ -z "${tool:-}" ] || [ -z "${ports:-}" ]; then
    return 1
  fi
  return 0
}

reset_firewall_changes() {
  if ! is_root; then
    die "--reset requires root. Re-run with sudo."
  fi
  if ! read_firewall_state; then
    warn "No firewall state found at $FIREWALL_STATE_FILE"
    return
  fi

  case "$tool" in
    ufw)
      reset_ufw "$ports"
      ;;
    firewalld)
      reset_firewalld "$ports"
      ;;
    iptables)
      reset_iptables "$ports"
      ;;
    *)
      warn "Unknown firewall tool in state: $tool"
      ;;
  esac

  rm -f "$FIREWALL_STATE_FILE"
  log "Firewall state cleared: $FIREWALL_STATE_FILE"
}

reset_port_map() {
  if ! read_portmap_state; then
    warn "No port-mapping state found at $PORTMAP_STATE_FILE"
    return
  fi

  local ports
  IFS=',' read -r -a ports <<< "$ports"
  case "$tool" in
    upnpc)
      if ! has_cmd upnpc; then
        warn "upnpc not found; cannot remove port mappings."
        return
      fi
      for p in "${ports[@]}"; do
        upnpc -d "$p" TCP >/dev/null 2>&1 || true
      done
      ;;
    natpmpc)
      if ! has_cmd natpmpc; then
        warn "natpmpc not found; cannot remove port mappings."
        return
      fi
      for p in "${ports[@]}"; do
        natpmpc -a "$p" "$p" tcp 0 >/dev/null 2>&1 || true
      done
      ;;
    *)
      warn "Unknown port-mapping tool in state: $tool"
      ;;
  esac

  rm -f "$PORTMAP_STATE_FILE"
  log "Port-mapping state cleared: $PORTMAP_STATE_FILE"
}

apply_firewall_changes() {
  if [ "$APPLY_CHANGES" != "true" ]; then
    return
  fi
  if ! is_root; then
    die "--apply requires root. Re-run with sudo."
  fi

  if has_cmd ufw && ufw status >/dev/null 2>&1; then
    apply_ufw "$PORTS"
    write_firewall_state "ufw" "$PORTS"
    return
  fi
  if has_cmd firewall-cmd && firewall-cmd --state >/dev/null 2>&1; then
    apply_firewalld "$PORTS"
    write_firewall_state "firewalld" "$PORTS"
    return
  fi
  if has_cmd iptables; then
    apply_iptables "$PORTS"
    write_firewall_state "iptables" "$PORTS"
    return
  fi

  warn "No supported firewall tools found. Nothing to apply."
}

attempt_port_mapping() {
  if [ "$PORT_MAP" != "true" ]; then
    return
  fi

  local ports
  IFS=',' read -r -a ports <<< "$PORTS"
  local lan_ip
  lan_ip="$(get_primary_lan_ip)"

  if has_cmd upnpc; then
    if [ -z "$lan_ip" ]; then
      warn "Cannot determine LAN IP for UPnP; skipping upnpc."
    else
      log "Attempting UPnP port mapping with upnpc..."
      for p in "${ports[@]}"; do
        upnpc -a "$lan_ip" "$p" "$p" TCP >/dev/null 2>&1 || true
      done
      write_portmap_state "upnpc" "$PORTS" "$lan_ip"
      return
    fi
  fi

  if has_cmd natpmpc; then
    log "Attempting NAT-PMP port mapping with natpmpc..."
    for p in "${ports[@]}"; do
      natpmpc -a "$p" "$p" tcp "$PORT_MAP_TTL" >/dev/null 2>&1 || true
    done
    write_portmap_state "natpmpc" "$PORTS" ""
    return
  fi

  warn "No UPnP/NAT-PMP tools found (upnpc, natpmpc)."
}

print_router_notes() {
  cat <<'EOF'

Router port-forwarding (manual step):
  - Forward TCP 8080 -> your machine's LAN IP
  - Forward TCP 9000 -> your machine's LAN IP

If your WAN IP is private (CGNAT), port-forwarding will not work. In that case,
WebRTC without TURN will likely fail for most friends.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --ports)
      PORTS="${2:-}"
      shift 2
      ;;
    --apply)
      APPLY_CHANGES="true"
      shift
      ;;
    --reset)
      RESET_CHANGES="true"
      shift
      ;;
    --port-map)
      PORT_MAP="true"
      shift
      ;;
    --port-map-ttl)
      PORT_MAP_TTL="${2:-}"
      shift 2
      ;;
    --check-listeners)
      CHECK_LISTENERS="true"
      shift
      ;;
    --no-wan-check)
      WAN_CHECK="false"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "Unknown option: $1"
      ;;
  esac
done

if [ -z "$PORT_MAP_TTL" ]; then
  PORT_MAP_TTL="3600"
fi

log "E-Goat network setup (local firewall + checks)"
if [ "$RESET_CHANGES" = "true" ]; then
  reset_firewall_changes
  reset_port_map
  exit 0
fi
check_os
check_lan_ip
check_wan_ip
apply_firewall_changes
attempt_port_mapping

if [ "$CHECK_LISTENERS" = "true" ]; then
  check_listeners "$PORTS"
fi

print_router_notes
get_primary_lan_ip() {
  if has_cmd ip; then
    local ip
    ip="$(ip route get 1.1.1.1 2>/dev/null | awk '/src/ {print $7; exit}')"
    if [ -n "$ip" ]; then
      echo "$ip"
      return
    fi
    ip="$(ip -4 addr show scope global | awk '/inet / {print $2}' | cut -d/ -f1 | head -n1)"
    if [ -n "$ip" ]; then
      echo "$ip"
      return
    fi
  fi
}
