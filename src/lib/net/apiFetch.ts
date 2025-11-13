// src/lib/net/apiFetch.ts
import { browser } from '$app/environment';
import { get } from 'svelte/store';
import { WEBUI_BASE_URL } from '$lib/constants';
import { socket } from '$lib/stores';

function toRequestUrl(input: string | URL): string {
  // If given a URL object, use it as-is
  const str = typeof input === 'string' ? input : input.toString();

  // If already absolute (http/https), leave it
  if (/^https?:\/\/.*/.test(str)) return str;

  // Otherwise, it's a relative or root-relative path; return as-is so fetch
  // uses the current origin (works with dev proxy, and sends cookies).
  return str;
}

function normalizeInit(init: RequestInit = {}) {
	const headers = new Headers(init.headers ?? {});
	// Do not force credentials here. With dev proxy, same-origin requests
	// will include cookies by default (credentials: 'same-origin').
	return { ...init, headers };
}

let backendOrigin: string | null | undefined;

function getBackendOrigin() {
	if (backendOrigin !== undefined) {
		return backendOrigin;
	}

	if (!browser) {
		backendOrigin = null;
		return backendOrigin;
	}

	try {
		backendOrigin = new URL(WEBUI_BASE_URL, window.location.origin).origin;
	} catch {
		backendOrigin = null;
	}

	return backendOrigin;
}

function shouldIncludeCredentials(url: string, init: RequestInit): RequestCredentials | null {
	if (!browser || init.credentials) {
		return null;
	}

	const targetOrigin = (() => {
		try {
			return new URL(url, window.location.origin).origin;
		} catch {
			return null;
		}
	})();

	if (!targetOrigin) {
		return null;
	}

	const origin = getBackendOrigin();
	return origin && targetOrigin === origin ? 'include' : null;
}

export async function apiFetch(input: string | URL, init: RequestInit = {}) {
	const s = get(socket);
	const id = s?.id ?? '';

	const url = toRequestUrl(input);
	const next = normalizeInit(init);
	const credentials = shouldIncludeCredentials(url, next);

	if (id) {
		(next.headers as Headers).set('sid', id);
	}
	if (credentials) {
		next.credentials = credentials;
	}
	return fetch(url, next);
}
