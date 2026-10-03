/* eslint-disable max-classes-per-file */
// This is a subset of, and the counterpart to errors.go

const maybeToJson = async (response) => {
  try {
    return await response.json();
  } catch (e) {
    return null;
  }
};

export class UnauthorizedError extends Error {
  constructor(status) {
    super("Unauthorized");
    this.status = status; // 401 (bad/expired credentials) or 403 (valid credentials, no topic access)
  }
}

export class UserExistsError extends Error {
  static CODE = 40901; // errHTTPConflictUserExists

  constructor() {
    super("Username already exists");
  }
}

export class TopicReservedError extends Error {
  static CODE = 40902; // errHTTPConflictTopicReserved

  constructor() {
    super("Topic already reserved");
  }
}

export class AccountActionLimitReachedError extends Error {
  static CODE = 42906; // errHTTPTooManyRequestsLimitAccountActions

  constructor() {
    super("Account action limit reached");
  }
}

export class IncorrectPasswordError extends Error {
  static CODE = 40026; // errHTTPBadRequestIncorrectPasswordConfirmation

  constructor() {
    super("Password incorrect");
  }
}

export class EmailVerificationCodeInvalidError extends Error {
  static CODE = 40051; // errHTTPBadRequestEmailVerificationCodeInvalid

  constructor() {
    super("Email verification link invalid or expired");
  }
}

export class EmailPrimaryElsewhereError extends Error {
  static CODE = 40908; // errHTTPConflictEmailPrimaryElsewhere

  constructor() {
    super("Email address is the recovery email on another account");
  }
}

export const throwAppError = async (response) => {
  if (response.status === 401 || response.status === 403) {
    console.log(`[Error] HTTP ${response.status}`, response);
    throw new UnauthorizedError(response.status);
  }
  const error = await maybeToJson(response);
  if (error?.code) {
    console.log(`[Error] HTTP ${response.status}, ntfy error ${error.code}: ${error.error || ""}`, response);
    if (error.code === UserExistsError.CODE) {
      throw new UserExistsError();
    } else if (error.code === TopicReservedError.CODE) {
      throw new TopicReservedError();
    } else if (error.code === AccountActionLimitReachedError.CODE) {
      throw new AccountActionLimitReachedError();
    } else if (error.code === IncorrectPasswordError.CODE) {
      throw new IncorrectPasswordError();
    } else if (error.code === EmailVerificationCodeInvalidError.CODE) {
      throw new EmailVerificationCodeInvalidError();
    } else if (error.code === EmailPrimaryElsewhereError.CODE) {
      throw new EmailPrimaryElsewhereError();
    } else if (error?.error) {
      throw new Error(`Error ${error.code}: ${error.error}`);
    }
  }
  console.log(`[Error] HTTP ${response.status}, not a ntfy error`, response);
  throw new Error(`Unexpected response ${response.status}`);
};

export const fetchOrThrow = async (url, options) => {
  const response = await fetch(url, options);
  if (response.status !== 200) {
    await throwAppError(response);
  }
  return response; // Promise!
};
