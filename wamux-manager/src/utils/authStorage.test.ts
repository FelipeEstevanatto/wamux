import { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import apiClient from '@/services/api/client';
import useAuthStore from '@/store/authStore';
import {
  AUTH_STORAGE_KEY,
  DASHBOARD_BASE_STORAGE_KEY,
  DASHBOARD_KEY_STORAGE_KEY,
} from './authStorage';

function seedCredentials() {
  localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify({ state: { apiKey: 'test-key' } }));
  localStorage.setItem(DASHBOARD_KEY_STORAGE_KEY, 'test-key');
  localStorage.setItem(DASHBOARD_BASE_STORAGE_KEY, 'http://api.test');
}

function expectDashboardCredentialsGone() {
  expect(localStorage.getItem(DASHBOARD_KEY_STORAGE_KEY)).toBeNull();
  expect(localStorage.getItem(DASHBOARD_BASE_STORAGE_KEY)).toBeNull();
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

  it('removes the dashboard copy of the API key on logout', () => {
    useAuthStore.getState().logout();

    expectDashboardCredentialsGone();
    expect(localStorage.getItem(AUTH_STORAGE_KEY) ?? '').not.toContain('test-key');
  });

  it('removes the dashboard copy of the API key on a 401', async () => {
    const rejected = apiClient.get('/instance/all', {
      adapter: (config: InternalAxiosRequestConfig) => {
        const response = { status: 401, data: {}, config } as AxiosResponse;
        return Promise.reject(new AxiosError('Unauthorized', '401', config, null, response));
      },
    });

    await expect(rejected).rejects.toBeInstanceOf(AxiosError);
    expectDashboardCredentialsGone();
    expect(localStorage.getItem(AUTH_STORAGE_KEY)).toBeNull();
  });
});
