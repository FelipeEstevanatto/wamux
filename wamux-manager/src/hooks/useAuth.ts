import useAuthStore from '@/store/authStore';

/**
 * useAuth Hook
 *
 * Convenient hook to access the auth store (API URL, API key, login/logout).
 */
function useAuth() {
  const authStore = useAuthStore();

  return {
    // State
    isAuthenticated: authStore.isAuthenticated,
    apiUrl: authStore.apiUrl,
    apiKey: authStore.apiKey,

    // Methods
    login: authStore.login,
    logout: authStore.logout,
    setApiUrl: authStore.setApiUrl,
    setApiKey: authStore.setApiKey,
  };
}

export default useAuth;
