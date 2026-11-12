/**
 * End-to-end test support.
 *
 * Maestro cannot receive a real Privy email code, so when `EXPO_PUBLIC_E2E=1`
 * the sign-in screen accepts a fixed code and records a local mock session
 * instead of contacting Privy. Real builds never take this path.
 */
export const E2E_ENABLED = process.env.EXPO_PUBLIC_E2E === '1';

/** Email and code the Maestro sign-in subflow types in. */
export const E2E_EMAIL = 'e2e@afika.test';
export const E2E_CODE = '000000';

let signedIn = false;

/** True when the E2E bypass completed a mock sign-in in this session. */
export function isE2eSignedIn(): boolean {
  return E2E_ENABLED && signedIn;
}

/** Records a mock sign-in so the boot screen skips the real sign-in gate. */
export function markE2eSignedIn(): void {
  signedIn = true;
}
