import { ApiError } from "./api";

export interface FormState {
  error?: string;
  saved?: boolean;
  fields?: Record<string, string>;
  /** Free text a form wants to show after success (a test result). */
  info?: string;
}

/** An API failure as form state. Anything else (a redirect to sign in, for one) is not ours to swallow. */
export function failure(e: unknown): FormState {
  if (!(e instanceof ApiError)) throw e;
  if (e.status === 403) return { error: "You do not have access to do that" };
  return { error: e.message, fields: e.fields };
}

/** The state of the form that makes an activation code: the code is shown once, from here. */
export interface ActivationState extends FormState {
  code?: string;
  expires?: string;
}
