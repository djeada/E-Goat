# 🔧 WebRTC Troubleshooting Guide for E-Goat

## 🎯 Quick Diagnosis

### ✅ **WebRTC Working Correctly**
You should see these indicators:

**In Browser Console (F12):**
```
🔌 Connected to signaling server
👋 [peer-id] joined the room  
🔄 Auto-connecting to [peer-id]...
🔗 WebRTC connecting with [peer-id]
🧊 ICE candidate for [peer-id]: candidate:...
📡 Data channel opened with [peer-id]
📡 ✅ Data channel ready with [peer-id] - WebRTC P2P active!
🚀 ✅ Sent via WebRTC to 1 peer(s)
📨 ✅ Message via WebRTC from [peer-id]
```

**In Debug Panel:**
- ✅ Connection Quality: "P2P Active (1 WebRTC channels)"
- ✅ WebRTC Channels: "[peer-id]: open"
- ✅ Connected Peers: 1 or more

---

## ❌ **Common Issues & Solutions**

### Issue 1: "Messages fall back to HTTP polling"

**Symptoms:**
- Messages show: "⚠️ No WebRTC channels available, using fallback"
- Connection Quality: "HTTP Polling Only" (red)
- Console shows: "⚠️ WebRTC not available: dataChannels=0"

**Root Cause:** WebRTC connection never established successfully

**Debug Steps:**
1. **Check Signaling Connection:**
   ```
   Look for: "🔌 Connected to signaling server"
   Missing? Check WebSocket connection to ws://localhost:9000/signal
   ```

2. **Check Peer Discovery:**
   ```
   Look for: "👋 [peer-id] joined the room"
   Missing? Peers may not be discovering each other
   ```

3. **Check WebRTC Negotiation:**
   ```
   Look for: "🔗 WebRTC connecting with [peer-id]"
   Missing? Auto-connection may have failed
   ```

**Solutions:**
- **Wait 15 seconds** for WebRTC negotiation to complete
- **Refresh both browser tabs** and retry
- **Try manual connection** using "Connect to Peer" button
- **Use 127.0.0.1 instead of localhost** (some systems work better)

---

### Issue 2: "Connection timeout" or "ICE connection failed"

**Symptoms:**
- Console shows: "⏰ Connection timeout with [peer-id], cleaning up..."
- Or: "❌ ICE connection failed with [peer-id]"
- Connection Quality stays "Transport Layer" (orange) or red

**Root Cause:** ICE connectivity check failed (network/firewall issues)

**Debug Steps:**
1. **Check ICE Candidates:**
   ```
   Look for: "🧊 ICE candidate for [peer-id]: candidate:..."
   Few/no candidates? STUN servers may be blocked
   ```

2. **Check ICE Connection State:**
   ```
   Look for: "🧊 ICE connection state with [peer-id]: connected"
   Shows "failed"? Network path is blocked
   ```

**Solutions:**
- **Corporate/Restricted Networks:** May block STUN/TURN traffic
- **Add More STUN Servers:** Edit `rtcConfig` in chat.js:
  ```javascript
  const rtcConfig = {
    iceServers: [
      { urls: 'stun:stun.l.google.com:19302' },
      { urls: 'stun:stun.cloudflare.com:3478' },
      { urls: 'stun:stun.nextcloud.com:443' }
    ]
  };
  ```
- **For Internet Connections:** Add TURN server (paid service required)

---

### Issue 3: "Data channel created but never opens"

**Symptoms:**
- Console shows: "📡 Setting up data channel with [peer-id], initial state: connecting"
- But never shows: "📡 Data channel opened with [peer-id]"
- Debug Panel shows: "[peer-id]: connecting" (never "open")

**Root Cause:** WebRTC peer connection succeeded but data channel setup failed

**Debug Steps:**
1. **Check Connection State:**
   ```
   Look for: "🚀 WebRTC P2P connected to [peer-id]"
   Missing? Peer connection itself failed
   ```

2. **Check Data Channel Events:**
   ```
   Look for data channel error messages
   May show timeout or connection issues
   ```

**Solutions:**
- **Browser Compatibility:** Try Chrome or Firefox (best WebRTC support)
- **Firewall Rules:** Ensure WebRTC UDP traffic is allowed
- **Refresh and Retry:** Data channel timing can be sensitive

---

### Issue 4: "Auto-connection doesn't work"

**Symptoms:**
- Peers join room but don't auto-connect
- No "👋 [peer-id] joined the room" messages
- Must use manual "Connect to Peer" button

**Root Cause:** Signaling server not broadcasting peer join events

**Debug Steps:**
1. **Check Server Logs:**
   ```bash
   # Look for peer registration messages in server console
   2025/07/13 09:59:25 🚀 Transport manager initialized for peer: instance-8080
   ```

2. **Check Signaling Messages:**
   ```
   Should see "📡 Signal from [peer-id]: peer_joined"
   Missing? Server-side issue
   ```

**Solutions:**
- **Restart Server:** `go run cmd/messanger/main.go`
- **Check Server Ports:** Ensure 8080 (HTTP) and 9000 (WebSocket) are available
- **Manual Connection:** Use peer IDs from debug panel

---

## 🌐 **Network Scenarios**

### 🏠 **Same Machine (Localhost)**
- ✅ **Should Work:** Basic WebRTC functionality
- ⚠️ **May Fail:** Some browsers restrict localhost WebRTC
- 🔧 **Try:** Use 127.0.0.1 instead of localhost

### 🏢 **Same LAN (Different Devices)**
- ✅ **Should Work:** Most reliable WebRTC scenario
- ⚠️ **May Fail:** Corporate firewalls blocking WebRTC
- 🔧 **Try:** Ensure devices can reach ports 8080/9000

### 🌍 **Internet (Different Networks)**
- ⚠️ **Needs TURN:** Most home/office routers require TURN for NAT traversal
- 💰 **Paid Service:** TURN servers usually cost money (Twilio, Google, etc.)
- 🔧 **Setup:** Add TURN credentials to `rtcConfig`

---

## 🔬 **Advanced Debugging**

### Enable Verbose WebRTC Logging

Add to browser console:
```javascript
// Enable detailed WebRTC logging
localStorage.setItem('debug', 'webrtc*');

// Or in chat.js, add:
console.log('WebRTC Config:', rtcConfig);
```

### Check Network Connectivity

Test STUN server connectivity:
```bash
# Test if STUN servers are reachable
curl -v telnet://stun.l.google.com:19302
```

### Monitor Data Channel Buffer

Watch for buffer issues:
```javascript
// In browser console, check channel buffer
for (const [peerID, channel] of dataChannels) {
  console.log(`${peerID}: ${channel.readyState}, buffer: ${channel.bufferedAmount}`);
}
```

---

## 🎯 **Testing Checklist**

Use this checklist to verify WebRTC functionality:

### ✅ **Basic Connectivity**
- [ ] Signaling server connects (`🔌 Connected to signaling server`)
- [ ] Peers discover each other (`👋 [peer] joined the room`)
- [ ] WebRTC negotiation starts (`🔗 WebRTC connecting`)

### ✅ **ICE Connectivity**
- [ ] ICE candidates exchange (`🧊 ICE candidate for [peer]`)
- [ ] ICE connection succeeds (`🧊 ICE connected with [peer]`)
- [ ] No connection timeouts (`⏰ Connection timeout`)

### ✅ **Data Channels**
- [ ] Data channels created (`📡 Setting up data channel`)
- [ ] Data channels open (`📡 Data channel opened`)
- [ ] Test messages work (`🔧 ✅ Connection test successful`)

### ✅ **Message Flow**
- [ ] Messages sent via WebRTC (`🚀 ✅ Sent via WebRTC`)
- [ ] Messages received via WebRTC (`📨 ✅ Message via WebRTC`)
- [ ] No HTTP fallback (`NOT: ⚠️ No WebRTC channels available`)

---

## 🏆 **Success Criteria**

**Your WebRTC is working perfectly when:**

1. **Debug Panel shows:** "P2P Active (1 WebRTC channels)" in green
2. **Messages route via WebRTC:** Not HTTP polling fallback
3. **Sub-second latency:** Near-instant message delivery
4. **Reliable connection:** Stays connected during normal use

**If you see orange "Transport Layer" or red "HTTP Polling Only":**
- The system is still working (messages get through!)
- But you're not getting the full P2P performance benefits
- Use this guide to troubleshoot the WebRTC connection

---

## 💡 **Pro Tips**

### For Development
- **Chrome DevTools → Application → Local Storage** to clear signaling state
- **Use different browser profiles** to simulate different users
- **Monitor Network tab** for WebSocket signaling traffic

### For Production
- **Add multiple STUN servers** for redundancy
- **Consider TURN server** for enterprise deployments
- **Monitor connection quality** to understand user experience

### For Different Environments
- **Localhost:** Use 127.0.0.1, expect some flakiness
- **LAN:** Most reliable, should work consistently  
- **Internet:** Requires TURN server for most users

The layered architecture ensures messaging always works, even when WebRTC doesn't cooperate! 🎉
