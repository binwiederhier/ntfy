// This is a subset of, and the counterpart to errors.go

export const fetchOrThrow = async (url, options) => {
  const response = await fetch(url, options);
  if (response.status !== 200) {
    await throwAppError(response);
  }
  return response; // Promise!
};

export const throwAppError = async (response) => {
  if (response.status === 401 || response.status === 403) {
    console.log(`[Error] HTTP ${response.status}`, response);
    throw new UnauthorizedError();
  }
  const error = await maybeToJson(response);
  if (error?.code) {
    console.log(
      `[Error] HTTP ${response.status}, ntfy error ${error.code}: ${
        error.error || ""
      }`,
      response
    );
    if (error.code === UserExistsError.CODE) {
      throw new UserExistsError();
    } else if (error.code === TopicReservedError.CODE) {
      throw new TopicReservedError();
    } else if (error.code === AccountCreateLimitReachedError.CODE) {
      throw new AccountCreateLimitReachedError();
    } else if (error.code === IncorrectPasswordError.CODE) {
      throw new IncorrectPasswordError();
    } else if (error?.error) {
      throw new Error(`Error ${error.code}: ${error.error}`);
    }
  }
  console.log(`[Error] HTTP ${response.status}, not a ntfy error`, response);
  throw new Error(`Unexpected response ${response.status}`);
};

const maybeToJson = async (response) => {
  try {
    return await response.json();
  } catch (e) {
    return null;
  }
};

export class UnauthorizedError extends Error {
  constructor() {
    super("Unauthorized");
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

export class AccountCreateLimitReachedError extends Error {
  static CODE = 42906; // errHTTPTooManyRequestsLimitAccountCreation
  constructor() {
    super("Account creation limit reached");
  }
}

export class IncorrectPasswordError extends Error {
  static CODE = 40026; // errHTTPBadRequestIncorrectPasswordConfirmation
  constructor() {
    super("Password incorrect");
  }
}
