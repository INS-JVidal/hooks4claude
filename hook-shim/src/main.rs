use std::io::{self, Read, Write};
use std::os::unix::net::UnixStream;
use std::time::Duration;

const MAX_STDIN: usize = 1 << 20; // 1 MiB
const MSG_EVENT: u8 = 0x01;
const MSG_CACHE_QUERY: u8 = 0x02;
const MSG_CACHE_RESPONSE: u8 = 0x03;

fn socket_path() -> String {
    std::env::var("HOOK_CLIENT_SOCK").unwrap_or_else(|_| "/tmp/hook-client.sock".to_string())
}

fn write_msg(stream: &mut UnixStream, msg_type: u8, payload: &[u8]) -> io::Result<()> {
    let mut hdr = [0u8; 5];
    hdr[0] = msg_type;
    let len = payload.len() as u32;
    hdr[1..5].copy_from_slice(&len.to_be_bytes());
    stream.write_all(&hdr)?;
    if !payload.is_empty() {
        stream.write_all(payload)?;
    }
    Ok(())
}

fn read_msg(stream: &mut UnixStream) -> io::Result<(u8, Vec<u8>)> {
    let mut hdr = [0u8; 5];
    stream.read_exact(&mut hdr)?;
    let msg_type = hdr[0];
    let len = u32::from_be_bytes([hdr[1], hdr[2], hdr[3], hdr[4]]) as usize;
    if len == 0 {
        return Ok((msg_type, Vec::new()));
    }
    let mut payload = vec![0u8; len];
    stream.read_exact(&mut payload)?;
    Ok((msg_type, payload))
}

fn main() {
    // Read stdin (bounded).
    let mut stdin_data = Vec::new();
    let _ = io::stdin().take(MAX_STDIN as u64).read_to_end(&mut stdin_data);
    if stdin_data.is_empty() {
        return;
    }

    // Connect to daemon.
    let sock = socket_path();
    let mut stream = match UnixStream::connect(&sock) {
        Ok(s) => s,
        Err(_) => return, // daemon not running — exit 0 silently
    };
    let _ = stream.set_read_timeout(Some(Duration::from_secs(2)));
    let _ = stream.set_write_timeout(Some(Duration::from_millis(500)));

    // Minimal JSON parsing: extract hook_event_name and tool_name.
    let is_pre_tool_use_read = match serde_json::from_slice::<serde_json::Value>(&stdin_data) {
        Ok(val) => {
            val.get("hook_event_name").and_then(|v| v.as_str()) == Some("PreToolUse")
                && val.get("tool_name").and_then(|v| v.as_str()) == Some("Read")
        }
        Err(_) => false,
    };

    if is_pre_tool_use_read {
        // Send as cache query and wait for response.
        if write_msg(&mut stream, MSG_CACHE_QUERY, &stdin_data).is_ok() {
            if let Ok((msg_type, payload)) = read_msg(&mut stream) {
                if msg_type == MSG_CACHE_RESPONSE && !payload.is_empty() {
                    // Check if response has hookSpecificOutput.
                    if let Ok(val) = serde_json::from_slice::<serde_json::Value>(&payload) {
                        if val.get("hookSpecificOutput").is_some() {
                            let _ = io::stdout().write_all(&payload);
                            let _ = io::stdout().write_all(b"\n");
                        }
                    }
                }
            }
        }
    } else {
        // Fire-and-forget event.
        let _ = write_msg(&mut stream, MSG_EVENT, &stdin_data);
    }
}
