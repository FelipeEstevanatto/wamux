export const AUTH_STORAGE_KEY = 'wamux-auth';

// Older versions kept copies of the API key under these names, and nothing reads
// them any more. Left alone they would outlive every sign-out.
const LEGACY_CREDENTIAL_KEYS = ['egogo_dash_key', 'egogo_dash_base', 'evolution-auth'];

export function clearStoredCredentials() {
  localStorage.removeItem(AUTH_STORAGE_KEY);
}

export function removeLegacyCredentials() {
  for (const key of LEGACY_CREDENTIAL_KEYS) localStorage.removeItem(key);
}
