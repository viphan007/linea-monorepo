export const isEmptyObject = (obj: object): boolean => {
  return Object.keys(obj).length === 0 && obj.constructor === Object;
};

export const isZero = (val: number | bigint): boolean => {
  return val === 0 || val === 0n;
};

export const isEmptyString = (val: string): boolean => {
  return val === "";
};

export const isNull = (value: unknown): value is null => {
  return value === null;
};

export const isUndefined = (value: unknown): value is undefined => {
  return value === undefined;
};

export const isUndefinedOrNull = (value: unknown): value is undefined | null => {
  return isUndefined(value) || isNull(value);
};

export const isUndefinedOrEmptyString = (value: string | undefined): value is undefined => {
  return isUndefined(value) || isEmptyString(value);
};

export const isHomePage = (pathname: string): boolean => {
  return pathname === "/";
};
