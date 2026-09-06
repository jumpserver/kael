# Command execution lifecycle

Koko owns connection-bound command executions. Luna relays calls and presents their progress. Kael runs the agent and limits how long it waits for each tool response.

## Wire contract

`execute_shell` and the other Koko command tools return after at most two seconds. A long operation returns an `execution_id`, its original `tool_call_id`, `status`, bounded `output`, `elapsed_ms`, `process_finished`, and `stop_confirmed`. The RPC is complete even when the command is still running. This releases the model to inspect other evidence.

The same terminal manifest supplies `get_command_execution`, `wait_command_execution`, and `cancel_command_execution`. Waiting defaults to 10 seconds and is capped at 30 seconds, returning early on completion or a detected input prompt. Ending or cancelling an observation wait neither cancels the original execution nor renews its deadline. These tools never rerun a command. Executions are private to one resource connection, with one occupied command slot and at most 32 retained receipts. Closing the connection cancels its executions; they do not survive reconnects or Koko restarts.

## Deadlines and cancellation

- Koko command execution defaults to 600 seconds. `timeout_seconds` can request up to 3600 seconds. This is a total execution deadline, not an inactivity or observation timeout. Command ACL review has a separate five-minute limit. Session policy can shorten the execution budget; Kael's run deadline (30 minutes by default) and 128-call budget still apply, so a one-hour command budget does not guarantee a one-hour agent run.
- Kael's `TOOL_RESULT_TIMEOUT` defaults to 45 seconds **per relayed RPC**, starting after Kael approval and dispatch. Receipt timeout requests cancellation and returns a structured unknown-execution error to the model. It does not terminate the entire agent run. A concurrently arriving terminal receipt wins if it commits first; late results cannot overwrite the timeout receipt.
- Browser and Electron Kael HTTP requests are bounded at 15 seconds. Result submissions retain the existing idempotent retry mechanism.
- Cancellation of an already-completed execute RPC still cancels its continuing execution. Luna retains this relationship and cancels remaining executions when a run ends or its session disconnects.
- SSH waits briefly for an exit notification after interrupt before closing its channel. PTY waits for a prompt after Ctrl+C. A local return is not proof that the remote process stopped. Unconfirmed stops remain unavailable for further command execution until reconnect; the model must not blindly repeat a write.
- Non-cooperative transports cannot occupy a model RPC indefinitely. After the cancellation grace period, status becomes `unknown`; the worker's slot remains reserved so retries cannot spawn unlimited blocked workers.

## Progress and recovery

Wait/status results include the last 16 KiB of partial output and update the original command's timeline entry in Luna. Errors preserve this structured output instead of replacing it with only `context deadline exceeded`. Command state and RPC completion are separate: a successful status query can describe an execution that is still running. Recognized password/confirmation/pager prompts are shown as `waiting_input`; this hint never authorizes entering credentials or answering the prompt automatically. Existing approval modes remain in force for get/wait/cancel tools.

After execution starts, snapshots also include `execution_elapsed_ms` (excluding ACL review), `output_idle_ms` (since last observed output, or execution start if silent), and `remaining_ms` (the original total execution budget). Thirty seconds without output sets the advisory `attention_reason=no_output`; it does not stop execution or establish process liveness. An input prompt sets `attention_reason=waiting_input`. Completed receipts freeze these timings and clear advisory attention. SSH continues publishing bounded progress after its retained output buffer fills; PTY timestamps actual received output, including identical redraws, rather than snapshot reads. Luna shows inactivity and remaining execution time on active commands.

Ordinary commands use the existing independent SSH execution path when allowed. PTY supports OSC 133/633 completion markers when the shell supplies them, and recognizes directory changes in conventional user@host prompts. Other terminal programs retain conservative prompt detection; unsupported prompts can still require cancellation or reconnection.

The agent reassesses partial evidence after every observation and decides whether to wait, inspect independent evidence, handle an authorized input through an available capability, or cancel. It prefers 30-second observations for known long or quiet work, avoids mistaking silence for failure, and explains meaningful progress to the user. It resolves ongoing executions before its final answer and reports unconfirmed outcomes honestly. If no authorized input capability exists, it cancels and checks termination before handing control back to the user. Progress refreshes when get/wait returns; it is not a continuous output push stream or simultaneous model inference during a blocked tool call.

## Validation

Regression checks cover early yielding, silence and observation cancellation without stopping execution or renewing its budget, prompt-triggered observation return, progress after buffer limits, repeated PTY redraw activity, connection isolation, partial timeout output, cancellation after the initial RPC completes, changed PTY prompts/split completion markers, Kael's missing-receipt deadline and late results, and Luna's original-command progress mapping. Live asset behavior still depends on the remote shell's prompt/integration support and SSH signal handling.
