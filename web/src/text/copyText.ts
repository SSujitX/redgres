export async function copyText(value: string): Promise<void> {
  if (value === "") {
    return;
  }
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch {
      // Fall through to execCommand for permission or transient failures.
    }
  }
  const area = document.createElement("textarea");
  area.value = value;
  area.setAttribute("readonly", "true");
  area.style.position = "fixed";
  area.style.left = "-9999px";
  document.body.appendChild(area);
  area.select();
  try {
    document.execCommand("copy");
  } finally {
    document.body.removeChild(area);
  }
}

/** Keeps a user-gesture alive while an async value is fetched (e.g. POST reveal). */
export async function copyTextFromPromise(getValue: () => Promise<string>): Promise<void> {
  if (navigator.clipboard?.write && typeof ClipboardItem !== "undefined") {
    await navigator.clipboard.write([
      new ClipboardItem({
        "text/plain": getValue().then((value) => new Blob([value], { type: "text/plain" })),
      }),
    ]);
    return;
  }
  await copyText(await getValue());
}
