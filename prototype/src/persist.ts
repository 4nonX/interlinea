// Keeps the prototype's work between visits: projects, translations,
// termbases, settings and the project memories. Everything is stored in
// the browser's IndexedDB for this site, so it belongs to one browser
// profile; nothing leaves the computer.
//
// Storage can be missing or refuse to work (private windows, blocked
// site data, a full disk). The prototype then keeps working in memory and
// says so in Settings.

const DB_NAME = "interlinea-prototype";
const STORE = "kv";

/** Bumped when the shape of the saved state changes incompatibly. */
export const STATE_VERSION = 1;

export type StorageStatus = "ok" | "unavailable" | "failed";

let status: StorageStatus = "ok";
let lastError = "";
let dbPromise: Promise<IDBDatabase> | null = null;

export function storageStatus(): { status: StorageStatus; error: string } {
  return { status, error: lastError };
}

function fail(s: StorageStatus, e: unknown) {
  status = s;
  lastError = e instanceof Error ? e.message : String(e);
}

function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise;
  dbPromise = new Promise<IDBDatabase>((resolve, reject) => {
    let req: IDBOpenDBRequest;
    try {
      if (typeof indexedDB === "undefined")
        throw new Error("this browser has no IndexedDB");
      req = indexedDB.open(DB_NAME, 1);
    } catch (e) {
      reject(e);
      return;
    }
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains(STORE))
        req.result.createObjectStore(STORE);
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () =>
      reject(req.error ?? new Error("IndexedDB could not be opened"));
    req.onblocked = () =>
      reject(new Error("IndexedDB is blocked by another tab"));
  });
  dbPromise.catch((e: unknown) => {
    dbPromise = null;
    fail("unavailable", e);
  });
  return dbPromise;
}

function run<T>(
  mode: IDBTransactionMode,
  fn: (s: IDBObjectStore) => IDBRequest<T> | void,
): Promise<T | undefined> {
  return openDB().then(
    (db) =>
      new Promise<T | undefined>((resolve, reject) => {
        let result: T | undefined;
        const tx = db.transaction(STORE, mode);
        const req = fn(tx.objectStore(STORE));
        if (req) req.onsuccess = () => (result = req.result);
        tx.oncomplete = () => resolve(result);
        tx.onerror = () =>
          reject(tx.error ?? new Error("IndexedDB transaction failed"));
        tx.onabort = () =>
          reject(tx.error ?? new Error("IndexedDB transaction aborted"));
      }),
  );
}

/** Reads a saved value; undefined when there is none or storage does not work. */
export async function load<T>(key: string): Promise<T | undefined> {
  try {
    return (
      (await run<T>("readonly", (s) => s.get(key) as IDBRequest<T>)) ??
      undefined
    );
  } catch (e) {
    fail("unavailable", e);
    return undefined;
  }
}

/** Saves a value; reports false when storage does not work. */
export async function save(key: string, value: unknown): Promise<boolean> {
  try {
    await run("readwrite", (s) => {
      s.put(value, key);
    });
    if (status === "failed") status = "ok";
    return true;
  } catch (e) {
    fail(status === "unavailable" ? "unavailable" : "failed", e);
    return false;
  }
}

export async function remove(key: string): Promise<void> {
  try {
    await run("readwrite", (s) => {
      s.delete(key);
    });
  } catch (e) {
    fail("failed", e);
  }
}

/** Deletes everything the prototype saved. */
export async function clearAll(): Promise<void> {
  try {
    await run("readwrite", (s) => {
      s.clear();
    });
  } catch (e) {
    fail("failed", e);
  }
}

/** All saved keys that start with prefix. */
export async function keys(prefix: string): Promise<string[]> {
  try {
    const all =
      (await run<IDBValidKey[]>("readonly", (s) => s.getAllKeys())) ?? [];
    return all.filter(
      (k): k is string => typeof k === "string" && k.startsWith(prefix),
    );
  } catch (e) {
    fail("unavailable", e);
    return [];
  }
}

/**
 * Whether this page load starts from the sample data: the address has
 * ?reset (as the browser tests use). The parameter is removed from the
 * address so a later reload keeps the work.
 */
export function takeResetRequest(): boolean {
  try {
    const url = new URL(location.href);
    if (!url.searchParams.has("reset")) return false;
    url.searchParams.delete("reset");
    history.replaceState(
      history.state,
      "",
      url.pathname + (url.search ? url.search : "") + url.hash,
    );
    return true;
  } catch {
    return false;
  }
}
