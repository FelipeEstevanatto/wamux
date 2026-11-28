export const AUTH_STORAGE_KEY = 'wamux-auth';

export function clearStoredCredentials() {
  localStorage.removeItem(AUTH_STORAGE_KEY);
}
