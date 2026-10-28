import json
from pathlib import Path
import time
import unittest
from unittest.mock import patch

import codex_gateway as gateway


class FakeCodex:
    def __init__(self, events):
        self.events = iter(events)
        self.work = Path("/tmp/artex-codex-test")
        self.calls = []
        self.sent = []
        self.thread_id = None
        self.turn_id = None
        self.closed = False

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.closed = True

    def initialize(self):
        pass

    def rpc(self, method, params):
        self.calls.append((method, params))
        if method == "thread/start":
            return {"thread": {"id": "thread-test"}}
        if method == "turn/start":
            return {"turn": {"id": "turn-test"}}
        return {}

    def receive(self, _):
        return next(self.events)

    def send(self, message):
        self.sent.append(message)


class AdapterTests(unittest.TestCase):
    def request(self, **fields):
        return {"model": "runtime-test", "messages": [{"role": "user", "content": "hello"}], **fields}

    def test_tool_handoff_is_not_executed_on_host(self):
        event = {"id": 51, "method": "item/tool/call", "params": {
            "tool": gateway.tool_name("Bash"), "callId": "call-1", "arguments": {"command": "echo test"}}}
        runtime = FakeCodex([event])
        body = self.request(tools=[{"type": "function", "function": {
            "name": "Bash", "parameters": {"type": "object", "properties": {"command": {"type": "string"}}}}}])
        with patch.object(gateway, "runtime_config", return_value={}), patch.object(gateway.subprocess, "Popen") as spawn:
            reply = gateway.complete(body, factory=lambda: runtime)
        call = reply["choices"][0]["message"]["tool_calls"][0]
        self.assertEqual(call["function"]["name"], "Bash")
        self.assertEqual(json.loads(call["function"]["arguments"]), {"command": "echo test"})
        self.assertEqual(reply["choices"][0]["finish_reason"], "tool_calls")
        self.assertTrue(runtime.closed)
        spawn.assert_not_called()
        self.assertFalse(runtime.sent)

    def test_tool_result_retains_call_linkage_and_native_history(self):
        body = self.request(messages=[
            {"role": "system", "content": "system instruction"},
            {"role": "user", "content": "check"},
            {"role": "assistant", "content": None, "tool_calls": [{"id": "call-7", "function": {"name": "Read", "arguments": '{"path":"test"}'}}]},
            {"role": "tool", "tool_call_id": "call-7", "content": "file result"},
        ])
        system, history, user_input, _, _ = gateway.prepare_request(body)
        self.assertEqual(system, "system instruction")
        self.assertEqual(history[-2]["call_id"], history[-1]["call_id"])
        self.assertEqual(history[-2]["name"], gateway.tool_name("Read"))
        self.assertEqual(history[-1]["output"], "file result")
        self.assertTrue(user_input)

    def test_unknown_runtime_tool_fails_closed(self):
        runtime = FakeCodex([{"id": 5, "method": "item/tool/call", "params": {"tool": "host_shell", "callId": "x"}}])
        with patch.object(gateway, "runtime_config", return_value={}):
            with self.assertRaisesRegex(gateway.RuntimeFailure, "outside the ARTEX"):
                gateway.complete(self.request(), factory=lambda: runtime)
        self.assertTrue(runtime.closed)

    def test_failed_turn_is_not_reported_as_success(self):
        runtime = FakeCodex([{"method": "turn/completed", "params": {"turn": {"status": "failed", "error": {"message": "model unavailable"}}}}])
        with patch.object(gateway, "runtime_config", return_value={}):
            with self.assertRaisesRegex(gateway.RuntimeFailure, "model unavailable"):
                gateway.complete(self.request(), factory=lambda: runtime)

    def test_cancelled_request_closes_owned_runtime(self):
        runtime = FakeCodex([])
        with patch.object(gateway, "runtime_config", return_value={}):
            with self.assertRaises(ConnectionAbortedError):
                gateway.complete(self.request(), disconnected=lambda: True, factory=lambda: runtime)
        self.assertTrue(runtime.closed)

    def test_host_api_keys_are_not_inherited(self):
        with patch.dict(gateway.os.environ, {"OPENAI_API_KEY": "not-a-real-key", "ANTHROPIC_API_KEY": "not-a-real-key", "ARTEX_TEST_MARKER": "keep"}):
            env = gateway.runtime_environment()
        self.assertNotIn("OPENAI_API_KEY", env)
        self.assertNotIn("ANTHROPIC_API_KEY", env)
        self.assertEqual(env["ARTEX_TEST_MARKER"], "keep")

    def test_sse_includes_native_tool_id_and_usage(self):
        reply = {"id": "chat-test", "created": int(time.time()), "model": "runtime-test",
                 "choices": [{"message": {"role": "assistant", "content": None, "tool_calls": [{"id": "call-9", "type": "function", "function": {"name": "Read", "arguments": "{}"}}]}, "finish_reason": "tool_calls"}],
                 "usage": {"prompt_tokens": 12, "completion_tokens": 4, "total_tokens": 16}}
        chunks = list(gateway.stream_chunks(reply))
        self.assertEqual(chunks[1]["choices"][0]["delta"]["tool_calls"][0]["id"], "call-9")
        self.assertEqual(chunks[-1]["usage"]["total_tokens"], 16)
        self.assertEqual(chunks[-1]["choices"][0]["finish_reason"], "tool_calls")

    def test_chatgpt_login_is_required(self):
        runtime = object.__new__(gateway.Codex)
        runtime.rpc = lambda method, params: {"account": {"type": "apiKey"}} if method == "account/read" else {}
        runtime.send = lambda _: None
        with self.assertRaisesRegex(gateway.RuntimeFailure, "logged in with ChatGPT"):
            runtime.initialize()


if __name__ == "__main__":
    unittest.main()
