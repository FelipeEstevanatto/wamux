export const AUTH_STORAGE_KEY = 'wamux-auth';

// The static dashboard reads its own copy of the API key, seeded by the Dashboard
// page, so a sign-out has to clear that copy as well.
export const DASHBOARD_KEY_STORAGE_KEY = 'egogo_dash_key';
export const DASHBOARD_BASE_STORAGE_KEY = 'egogo_dash_base';

export function clearStoredCredentials() {
  localStorage.removeItem(AUTH_STORAGE_KEY);
  localStorage.removeItem(DASHBOARD_KEY_STORAGE_KEY);
  localStorage.removeItem(DASHBOARD_BASE_STORAGE_KEY);
}
