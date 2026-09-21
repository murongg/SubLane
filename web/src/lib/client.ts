export const clientProtocols = ['codex', 'claude', 'gemini'] as const
export type ClientProtocol = (typeof clientProtocols)[number]

// Model IDs are user input. Quote the complete JSON/URL argument before copying a shell command.
const shellLiteral = (value: string) => `'${value.replaceAll("'", "'\"'\"'")}'`

export function clientConfiguration(
  protocol: ClientProtocol,
  origin: string,
  model: string,
) {
  const baseURL = protocol === 'codex' ? `${origin}/v1` : origin
  const selectedModel = model.trim()
  if (protocol === 'codex') {
    return {
      baseURL,
      configuration: `${selectedModel ? `model = ${JSON.stringify(selectedModel)}\n` : ''}model_provider = "sublane"\n\n[model_providers.sublane]\nname = "SubLane"\nbase_url = ${JSON.stringify(baseURL)}\nenv_key = "SUBLANE_API_KEY"\nwire_api = "responses"\nrequires_openai_auth = false\nsupports_websockets = true`,
    }
  }
  const id = selectedModel || 'YOUR_MODEL_ID'
  const url =
    protocol === 'claude'
      ? `${origin}/v1/messages`
      : `${origin}/v1beta/models/${encodeURIComponent(id)}:generateContent`
  const body =
    protocol === 'claude'
      ? {
          model: id,
          max_tokens: 1024,
          messages: [{ role: 'user', content: 'Hello' }],
        }
      : { contents: [{ role: 'user', parts: [{ text: 'Hello' }] }] }
  const header = protocol === 'claude' ? 'x-api-key' : 'x-goog-api-key'
  return {
    baseURL,
    configuration: [
      `curl ${shellLiteral(url)}`,
      `  -H "${header}: $SUBLANE_API_KEY"`,
      ...(protocol === 'claude'
        ? ["  -H 'anthropic-version: 2023-06-01'"]
        : []),
      "  -H 'content-type: application/json'",
      `  --data ${shellLiteral(JSON.stringify(body))}`,
    ].join(' \\\n'),
  }
}
