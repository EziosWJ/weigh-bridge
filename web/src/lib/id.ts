/**
 * Generates a UUID-like identifier without requiring a secure browser context.
 *
 * `crypto.randomUUID()` is only exposed on HTTPS (or localhost). The plant
 * deployment is intentionally HTTP-only on the LAN, so use getRandomValues()
 * (available in insecure contexts) with a final non-cryptographic fallback.
 */
export function createId(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    try {
      return globalThis.crypto.randomUUID();
    } catch {
      // Fall through to the insecure-context-compatible implementation.
    }
  }

  const bytes = new Uint8Array(16);
  if (typeof globalThis.crypto?.getRandomValues === 'function') {
    globalThis.crypto.getRandomValues(bytes);
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256);
    }
  }

  // RFC 4122 version 4 and variant bits.
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;

  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
