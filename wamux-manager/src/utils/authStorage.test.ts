import { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import apiClient from '@/services/api/client';
import useAuthStore from '@/store/authStore';
import { AUTH_STORAGE_KEY, removeLegacyCredentials } from './authStorage';

function seedCredentials() {
  localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify({ state: { apiKey: 'test-key' } }));
}

describe('stored credentials', () => {
  beforeEach(() => {
    // Both paths redirect to the login page, which jsdom cannot navigate to.
    vi.stubGlobal('location', { href: '', origin: 'http://localhost' });
    vi.spyOn(console, 'error').mockImplementation(() => {});
    localStorage.clear();
    seedCredentials();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('removes the API key on logout', () => {
    useAuthStore.getState().logout();

    expect(localStorage.getItem(AUTH_STORAGE_KEY) ?? '').not.toContain('test-key');
  });

  it('removes the API key on a 401', async () => {
    const rejected = apiClient.get('/instance/all', {
      adapter: (config: InternalAxiosRequestConfig) => {
        const response = { status: 401, data: {}, config } as AxiosResponse;
        return Promise.reject(new AxiosError('Unauthorized', '401', config, null, response));
      },
    });

    await expect(rejected).rejects.toBeInstanceOf(AxiosError);
    expect(localStorage.getItem(AUTH_STORAGE_KEY)).toBeNull();
  });

  it('removes copies of the key left by older versions, and only those', () => {
    localStorage.setItem('egogo_dash_key', 'old-key');
    localStorage.setItem('egogo_dash_base', 'http://api.test');
    localStorage.setItem('evolution-auth', '{"state":{"apiKey":"old-key"}}');

    removeLegacyCredentials();

    expect(localStorage.getItem('egogo_dash_key')).toBeNull();
    expect(localStorage.getItem('egogo_dash_base')).toBeNull();
    expect(localStorage.getItem('evolution-auth')).toBeNull();
    expect(localStorage.getItem(AUTH_STORAGE_KEY)).not.toBeNull();
  });
});
