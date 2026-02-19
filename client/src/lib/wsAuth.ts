function toBase64URL(input: string): string {
  if (input.trim() === '') return ''
  const bytes = new TextEncoder().encode(input)
  let binary = ''
  for (const b of bytes) {
    binary += String.fromCharCode(b)
  }
  return btoa(binary)
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '')
}

export function buildWSProtocols(token: string): Array<string> {
  const protocols = ['sentinel.v1']
  const trimmed = token.trim()
  if (trimmed === '') return protocols
  const encoded = toBase64URL(trimmed)
  if (encoded !== '') {
    protocols.push(`sentinel.auth.${encoded}`)
  }
  return protocols
}
