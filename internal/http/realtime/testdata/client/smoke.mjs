import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";

// Browser-compatible native WebSocket API: no Socket.IO or npm package needed.
const ws = new WebSocket(process.env.CHAT_TEST_URL.replace(/^http/, "ws") + "/api/v1/ws");
const pending = new Map();
const timeout = setTimeout(() => process.exit(1), 10000);
let resolveReady;
let rejectReady;
const ready = new Promise((resolve, reject) => { resolveReady = resolve; rejectReady = reject; });
ws.onopen = () => ws.send(JSON.stringify({ event: "auth", data: { token: process.env.CHAT_TEST_TOKEN } }));
ws.onerror = () => rejectReady(new Error("connection failed"));
ws.onmessage = ({ data }) => {
  const frame = JSON.parse(data);
  if (frame.event === "auth:ok") resolveReady(frame);
  else if (frame.event === "ack") {
    pending.get(frame.requestId)?.(frame);
    pending.delete(frame.requestId);
  }
};
function request(event, data) {
  const requestId = randomUUID();
  return new Promise((resolve) => {
    pending.set(requestId, resolve);
    ws.send(JSON.stringify({ event, requestId, data }));
  });
}
try {
  const auth = await ready;
  assert.equal(auth.data.content.userId, process.env.CHAT_TEST_USER);
  const ack = await request("chat:send", {
    conversationId: randomUUID(), clientMessageId: randomUUID(), body: "Native WebSocket test",
  });
  assert.equal(ack.statusCode, 200);
  assert.equal(ack.data.content.senderId, process.env.CHAT_TEST_USER);
  const invalid = await request("chat:send", {});
  assert.equal(invalid.statusCode, 400);
} catch {
  // Never print tokens or connection options on test failure.
  process.exitCode = 1;
} finally {
  clearTimeout(timeout);
  ws.close();
}
