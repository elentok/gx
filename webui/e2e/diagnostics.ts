/**
 * Runs `run`; when it throws, re-throws an error that also holds what `read` saw, so a failure in a
 * log names the last state of the system. Diagnostics only: it never retries or waits.
 */
export async function withState<T>(
  label: string,
  read: () => Promise<string>,
  run: () => Promise<T>,
): Promise<T> {
  try {
    return await run()
  } catch (error) {
    let state: string
    try {
      state = await read()
    } catch (readError) {
      state = `(reading the state failed: ${readError instanceof Error ? readError.message : String(readError)})`
    }
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`${label}: ${message}\n\nlast state read:\n${state}`, { cause: error })
  }
}
