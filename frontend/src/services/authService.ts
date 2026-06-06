import { exchangeConfig } from './exchangeService';

export interface AuthUser {
  sub: string;
  email?: string;
  name?: string;
  roles?: string[];
  exp?: string;
}

export interface AuthStatus {
  enabled: boolean;
  provider: string;
}

export interface AuthSession {
  authenticated: boolean;
  enabled?: boolean;
  user?: AuthUser;
}

export interface AuthLogout {
  authenticated: false;
  logout_url?: string;
}

export async function fetchAuthStatus(): Promise<AuthStatus> {
  return authJSON<AuthStatus>('/auth/oidc/status');
}

export async function fetchAuthSession(): Promise<AuthSession> {
  try {
    return await authJSON<AuthSession>('/auth/me');
  } catch (err) {
    if (err instanceof AuthAPIError && err.status === 401) {
      return { authenticated: false, enabled: true };
    }
    throw err;
  }
}

export async function logout(): Promise<AuthLogout> {
  const redirect = typeof window === 'undefined' ? '/' : window.location.origin + window.location.pathname;
  const query = new URLSearchParams({ redirect });
  return authJSON<AuthLogout>(`/auth/logout?${query.toString()}`, { method: 'POST' });
}

export function oidcLoginURL(): string {
  const redirect = typeof window === 'undefined' ? '/' : window.location.origin + window.location.pathname;
  const query = new URLSearchParams({ redirect });
  return `${exchangeConfig.apiBaseURL}/auth/oidc/login?${query.toString()}`;
}

async function authJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${exchangeConfig.apiBaseURL}${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new AuthAPIError(response.status, payload.error || `Auth API error ${response.status}`);
  }

  return response.json() as Promise<T>;
}

class AuthAPIError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
    this.name = 'AuthAPIError';
  }
}
