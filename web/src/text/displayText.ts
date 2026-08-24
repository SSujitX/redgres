const BIDI_CONTROLS =
  /[\u061C\u200E\u200F\u202A\u202B\u202C\u202D\u202E\u2066\u2067\u2068\u2069]/gu;
const FORMAT_AND_CONTROL = /\p{Cf}|\p{Cc}/gu;
const REPLACEMENT = "\uFFFD";

export function displayText(value: string): string {
  return value.replace(BIDI_CONTROLS, REPLACEMENT).replace(FORMAT_AND_CONTROL, REPLACEMENT);
}
