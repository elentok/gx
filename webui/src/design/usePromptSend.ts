import type { SessionInfo, SessionPromptRequest } from "../generated/server";
import { ReasonStale } from "../ticket/answer";
import { useWrite } from "../useWrite";
import { postPrompt, type PostPrompt } from "./prompt";
import { promptable } from "./view";

/** The send call both composers share: one guarded prompt post that refetches the chat when the conversation moved. */
export function usePromptSend({
  session,
  lastMessage,
  connected,
  post = postPrompt,
  onStale,
  toastRefusal,
}: {
  session: SessionInfo;
  /** The id of the last chat message the page saw. */
  lastMessage?: string;
  connected: boolean;
  post?: PostPrompt;
  onStale?: () => void;
  /** Whether a refusal also toasts; off, it shows inline only. */
  toastRefusal: boolean;
}) {
  const { run: write, pending, refusal, error } = useWrite({ toastRefusal });
  const enabled = connected && promptable(session) && !pending;
  const send = (body: Partial<SessionPromptRequest>, onSent?: () => void) => {
    if (!enabled) return;
    return write(
      async () => {
        const res = await post({
          id: session.id,
          text: "",
          last_message: lastMessage,
          ...body,
        });
        if (res.refused && res.reason === ReasonStale) onStale?.();
        return res;
      },
      { onDone: onSent },
    );
  };
  return { send, write, enabled, pending, refusal, error };
}
