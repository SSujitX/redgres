import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { fetchOperation } from "../../api/operations";
import {
  createPostgresDatabase,
  deletePostgresRows,
  dropPostgresDatabase,
  duplicatePostgresDatabase,
  errorMessage,
  fetchPostgresConnection,
  fetchPostgresDatabase,
  fetchPostgresDatabases,
  fetchPostgresPrimaryKey,
  fetchPostgresRows,
  fetchPostgresTables,
  revealPostgresConnection,
  rotatePostgresCredentials,
  truncatePostgresDatabase,
  type DatabaseDetails,
  type DatabaseListItem,
  type RowPage,
  type TableItem,
} from "../../api/postgres";
import CredentialTicket, { type ShownCredential } from "../redis/CredentialTicket";
import CreateDatabaseForm from "./CreateDatabaseForm";
import DeleteSelectedRowsDialog from "./DeleteSelectedRowsDialog";
import DropDatabaseDialog from "./DropDatabaseDialog";
import DuplicateDatabaseForm from "./DuplicateDatabaseForm";
import RotatePasswordDialog from "./RotatePasswordDialog";
import TruncateProjectDataDialog from "./TruncateProjectDataDialog";
import { displayText } from "../../text/displayText";

const maxRowQueryRunes = 128;
const sessionExpired = "Your session has expired. Sign in again to continue.";
const postgresUnavailable = "PostgreSQL is unavailable";
const vaultOutOfSyncCopy =
  "The PostgreSQL password was changed but the vault could not be saved. Rotate again.";
const isolationRollbackCopy =
  "The source database ownership or CONNECT ACL changed during duplicate. The clone was rolled back.";

type ConnectionUrls = {
  savedCredentialStatus: string;
  maskedDirectUrl: string | null;
  maskedPooledUrl: string | null;
};

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
  }
}

function presentUrl(value: unknown): string | null {
  return typeof value === "string" && value !== "" ? value : null;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function stringField(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  return typeof value === "string" ? value : "";
}

function parsePostgresCredential(raw: unknown): ShownCredential | null {
  const record = asRecord(raw);
  if (!record) {
    return null;
  }
  const username = stringField(record, "username");
  const password = stringField(record, "password");
  if (username === "" || password === "") {
    return null;
  }
  const urls = asRecord(record.urls);
  const directUrl = urls ? stringField(urls, "direct") : "";
  const pooledUrl = urls ? stringField(urls, "pooled") : "";
  const shown: ShownCredential = { username, password };
  if (directUrl !== "") {
    shown.directUrl = directUrl;
  }
  if (pooledUrl !== "") {
    shown.pooledUrl = pooledUrl;
  }
  return shown;
}

function rotationEligible(details: DatabaseDetails | null): boolean {
  if (!details) {
    return false;
  }
  const owner = details.owner ?? "";
  if (owner === "") {
    return false;
  }
  return details.security?.owner_can_login === true && details.security?.owner_is_superuser === false;
}

type SelectedTable = {
  schema: string;
  name: string;
};

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

function isOperationId(value: string): boolean {
  return /^[0-9a-f]{32}$/.test(value);
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException("The operation was aborted.", "AbortError"));
      return;
    }
    const timer = window.setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    function onAbort() {
      window.clearTimeout(timer);
      reject(new DOMException("The operation was aborted.", "AbortError"));
    }
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

function queryRuneCount(value: string): number {
  return Array.from(value).length;
}

function pkScalar(value: unknown): string | number | boolean | null {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return value;
  }
  return null;
}

function pkKey(value: string | number | boolean): string {
  return JSON.stringify(value);
}

type DatabasesPageProps = {
  csrf?: string;
  focusDatabase?: string | null;
  focusNonce?: number;
  openCreate?: boolean;
};

export default function DatabasesPage({
  csrf = "",
  focusDatabase = null,
  focusNonce = 0,
  openCreate = false,
}: DatabasesPageProps) {
  const [items, setItems] = useState<DatabaseListItem[] | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [listError, setListError] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [details, setDetails] = useState<DatabaseDetails | null>(null);
  const [detailsError, setDetailsError] = useState("");
  const [loadingDetails, setLoadingDetails] = useState(false);
  const [connection, setConnection] = useState<ConnectionUrls | null>(null);
  const [connectionError, setConnectionError] = useState("");
  const [loadingConnection, setLoadingConnection] = useState(false);
  const [ticket, setTicket] = useState<ShownCredential | null>(null);
  const [ticketRotateWarning, setTicketRotateWarning] = useState(false);
  const [revealing, setRevealing] = useState(false);
  const [revealError, setRevealError] = useState("");
  const [rotateOpen, setRotateOpen] = useState(false);
  const [rotating, setRotating] = useState(false);
  const [rotateError, setRotateError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");
  const [duplicateOpen, setDuplicateOpen] = useState(false);
  const [duplicating, setDuplicating] = useState(false);
  const [duplicateError, setDuplicateError] = useState("");
  const [duplicateProgress, setDuplicateProgress] = useState<{ id: string; status: string } | null>(null);
  const [pendingSelect, setPendingSelect] = useState<string | null>(null);
  const revealAbort = useRef<AbortController | null>(null);
  const rotateAbort = useRef<AbortController | null>(null);
  const duplicateAbort = useRef<AbortController | null>(null);
  const listAbort = useRef<AbortController | null>(null);
  const createOpened = useRef(false);
  const [tables, setTables] = useState<TableItem[] | null>(null);
  const [tablesError, setTablesError] = useState("");
  const [tablesTruncated, setTablesTruncated] = useState(false);
  const [loadingTables, setLoadingTables] = useState(false);
  const [selectedTable, setSelectedTable] = useState<SelectedTable | null>(null);
  const [rowPage, setRowPage] = useState<RowPage | null>(null);
  const [rowsError, setRowsError] = useState("");
  const [rowsQueryError, setRowsQueryError] = useState("");
  const [loadingRows, setLoadingRows] = useState(false);
  const [queryDraft, setQueryDraft] = useState("");
  const [appliedQuery, setAppliedQuery] = useState("");
  const [primaryKey, setPrimaryKey] = useState<string[]>([]);
  const [selectedPks, setSelectedPks] = useState<Map<string, string | number | boolean>>(() => new Map());
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [deletePassword, setDeletePassword] = useState("");
  const [deleteError, setDeleteError] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [truncateOpen, setTruncateOpen] = useState(false);
  const [truncateConfirmation, setTruncateConfirmation] = useState("");
  const [truncatePassword, setTruncatePassword] = useState("");
  const [truncateError, setTruncateError] = useState("");
  const [truncating, setTruncating] = useState(false);
  const [dropOpen, setDropOpen] = useState(false);
  const [dropConfirmation, setDropConfirmation] = useState("");
  const [dropPassword, setDropPassword] = useState("");
  const [dropError, setDropError] = useState("");
  const [dropping, setDropping] = useState(false);
  const selectionAbort = useRef<AbortController | null>(null);
  const rowsAbort = useRef<AbortController | null>(null);
  const deleteAbort = useRef<AbortController | null>(null);
  const truncateAbort = useRef<AbortController | null>(null);
  const dropAbort = useRef<AbortController | null>(null);
  const rowsRegionRef = useRef<HTMLElement | null>(null);

  async function loadList(controller: AbortController) {
    try {
      const result = await fetchPostgresDatabases({ signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        clearTicket();
        setPendingSelect(null);
        setCreateOpen(false);
        setCreateError("");
        setItems(null);
        setListError(sessionExpired);
        return;
      }
      if (result.status === 200 && Array.isArray(result.body.databases)) {
        setItems(result.body.databases);
        setTruncated(result.body.truncated === true);
        setListError("");
        return;
      }
      setItems(null);
      setListError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setItems(null);
      setListError(postgresUnavailable);
    }
  }

  useEffect(() => {
    const controller = new AbortController();
    listAbort.current = controller;
    void loadList(controller);
    return () => {
      controller.abort();
      selectionAbort.current?.abort();
      rowsAbort.current?.abort();
      revealAbort.current?.abort();
      rotateAbort.current?.abort();
      duplicateAbort.current?.abort();
      deleteAbort.current?.abort();
      truncateAbort.current?.abort();
      dropAbort.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (!openCreate) {
      createOpened.current = false;
      return;
    }
    if (createOpened.current) {
      return;
    }
    if (items !== null && listError === "" && ticket === null) {
      setCreateOpen(true);
      createOpened.current = true;
    }
  }, [openCreate, items, listError, ticket]);

  useEffect(() => {
    if (!selectedTable) {
      return;
    }
    const node = rowsRegionRef.current;
    if (node && typeof node.scrollIntoView === "function") {
      node.scrollIntoView({ block: "nearest", inline: "nearest" });
    }
  }, [selectedTable]);

  function clearDeleteSecrets() {
    setDeleteOpen(false);
    setDeleteConfirmation("");
    setDeletePassword("");
    setDeleteError("");
    setDeleting(false);
  }

  function clearTruncateSecrets() {
    setTruncateOpen(false);
    setTruncateConfirmation("");
    setTruncatePassword("");
    setTruncateError("");
    setTruncating(false);
  }

  function clearDropSecrets() {
    setDropOpen(false);
    setDropConfirmation("");
    setDropPassword("");
    setDropError("");
    setDropping(false);
  }

  function clearRowSelection() {
    setSelectedPks(new Map());
    clearDeleteSecrets();
  }

  function clearRowState() {
    setSelectedTable(null);
    setRowPage(null);
    setRowsError("");
    setRowsQueryError("");
    setQueryDraft("");
    setAppliedQuery("");
    setLoadingRows(false);
    setPrimaryKey([]);
    clearRowSelection();
  }

  function clearTicket() {
    setTicket(null);
    setTicketRotateWarning(false);
    setRevealError("");
    setRevealing(false);
    setRotateOpen(false);
    setRotateError("");
    setRotating(false);
    setDuplicateOpen(false);
    setDuplicateError("");
    setDuplicating(false);
    setDuplicateProgress(null);
  }

  function openDetails(name: string) {
    selectionAbort.current?.abort();
    rowsAbort.current?.abort();
    revealAbort.current?.abort();
    rotateAbort.current?.abort();
    duplicateAbort.current?.abort();
    deleteAbort.current?.abort();
    truncateAbort.current?.abort();
    dropAbort.current?.abort();
    const controller = new AbortController();
    selectionAbort.current = controller;
    setSelected(name);
    setDetails(null);
    setDetailsError("");
    setLoadingDetails(true);
    setConnection(null);
    setConnectionError("");
    setLoadingConnection(true);
    setTables(null);
    setTablesError("");
    setTablesTruncated(false);
    setLoadingTables(true);
    setPendingSelect(null);
    clearTicket();
    clearTruncateSecrets();
    clearDropSecrets();
    clearRowState();
    void loadDetails(name, controller);
    void loadConnection(name, controller);
    void loadTables(name, controller);
  }

  useEffect(() => {
    if (!focusDatabase) {
      return;
    }
    openDetails(focusDatabase);
  }, [focusDatabase, focusNonce]);

  async function loadDetails(name: string, controller: AbortController) {
    try {
      const result = await fetchPostgresDatabase(name, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && result.body.database?.name === name) {
        setDetails(result.body.database);
        setDetailsError("");
        return;
      }
      setDetails(null);
      setDetailsError(errorMessage(result.body, "Database details are unavailable"));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setDetails(null);
      setDetailsError("PostgreSQL is unavailable");
    } finally {
      if (!controller.signal.aborted) {
        setLoadingDetails(false);
      }
    }
  }

  async function loadConnection(name: string, controller: AbortController) {
    try {
      const result = await fetchPostgresConnection(name, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setConnection(null);
        setConnectionError(sessionExpired);
        return;
      }
      if (result.status === 200) {
        setConnection({
          savedCredentialStatus: result.body.saved_credential?.status ?? "",
          maskedDirectUrl: presentUrl(result.body.masked_direct_url),
          maskedPooledUrl: presentUrl(result.body.masked_pooled_url),
        });
        setConnectionError("");
        return;
      }
      setConnection(null);
      setConnectionError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setConnection(null);
      setConnectionError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setLoadingConnection(false);
      }
    }
  }

  async function handleReveal() {
    if (!selected || revealing || rotating || creating || duplicating || truncating || dropping || ticket) {
      return;
    }
    revealAbort.current?.abort();
    const controller = new AbortController();
    revealAbort.current = controller;
    setRevealing(true);
    setRevealError("");
    try {
      const result = await revealPostgresConnection(selected, csrf, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setTicket(null);
        setRevealError(sessionExpired);
        return;
      }
      if (result.status === 404) {
        setTicket(null);
        setRevealError(errorMessage(result.body, "Not found"));
        return;
      }
      if (result.status === 200) {
        const shown = parsePostgresCredential(result.body.credential);
        if (!shown) {
          setRevealError(errorMessage(result.body, postgresUnavailable));
          return;
        }
        setTicket(shown);
        setTicketRotateWarning(false);
        setRevealError("");
        return;
      }
      setTicket(null);
      setRevealError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setTicket(null);
      setRevealError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setRevealing(false);
      }
    }
  }

  async function handleRotate(confirmation: string) {
    if (!selected || rotating || revealing || creating || duplicating || truncating || dropping || ticket) {
      return;
    }
    rotateAbort.current?.abort();
    const controller = new AbortController();
    rotateAbort.current = controller;
    setRotating(true);
    setRotateError("");
    try {
      const result = await rotatePostgresCredentials(selected, confirmation, csrf, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setRotateOpen(false);
        setRotateError("");
        setTicket(null);
        setTicketRotateWarning(false);
        setRevealError(sessionExpired);
        return;
      }
      if (result.status === 400 || result.status === 403) {
        setRotateError(
          errorMessage(
            result.body,
            result.status === 403
              ? "This PostgreSQL name is protected"
              : "Type the database name exactly to confirm rotation",
          ),
        );
        return;
      }
      if (result.status === 404) {
        setRotateOpen(false);
        setRotateError("");
        setTicket(null);
        setRevealError(errorMessage(result.body, "Not found"));
        return;
      }
      if (result.status === 503) {
        const copy = errorMessage(result.body, postgresUnavailable);
        if (copy === vaultOutOfSyncCopy) {
          setRotateError(vaultOutOfSyncCopy);
          return;
        }
        setRotateOpen(false);
        setRotateError("");
        setTicket(null);
        setRevealError(errorMessage(result.body, postgresUnavailable));
        return;
      }
      if (result.status === 200) {
        const shown = parsePostgresCredential(result.body.credential);
        if (!shown) {
          setRotateError(errorMessage(result.body, postgresUnavailable));
          return;
        }
        setRotateOpen(false);
        setRotateError("");
        setTicketRotateWarning(true);
        setTicket(shown);
        return;
      }
      setRotateError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setRotateError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setRotating(false);
      }
    }
  }

  function refreshList() {
    listAbort.current?.abort();
    const controller = new AbortController();
    listAbort.current = controller;
    void loadList(controller);
  }

  function dismissTicket() {
    const name = pendingSelect;
    clearTicket();
    setPendingSelect(null);
    if (name) {
      openDetails(name);
    }
  }

  async function handleCreate(database: string, owner: string) {
    if (ticket !== null || duplicating || truncating || dropping) {
      return;
    }
    setCreating(true);
    setCreateError("");
    try {
      const result = await createPostgresDatabase(database, owner, csrf);
      if (result.status === 401) {
        clearTicket();
        setPendingSelect(null);
        setCreateOpen(false);
        setCreateError("");
        setItems(null);
        setListError(sessionExpired);
        return;
      }
      if (result.status === 201) {
        const shown = parsePostgresCredential(result.body.credential);
        if (!shown) {
          setCreateError(errorMessage(result.body, postgresUnavailable));
          return;
        }
        const createdName =
          typeof result.body.resource?.name === "string" && result.body.resource.name !== ""
            ? result.body.resource.name
            : database;
        setCreateOpen(false);
        setCreateError("");
        setTicketRotateWarning(false);
        setTicket(shown);
        setPendingSelect(createdName);
        refreshList();
        return;
      }
      setCreateError(errorMessage(result.body, postgresUnavailable));
    } catch {
      setCreateError(postgresUnavailable);
    } finally {
      setCreating(false);
    }
  }

  function expireDuplicateSession() {
    setDuplicateOpen(false);
    setDuplicateError("");
    setDuplicateProgress(null);
    setTicket(null);
    setTicketRotateWarning(false);
    setRevealError(sessionExpired);
  }

  async function pollDuplicateOperation(
    operationId: string,
    submittedName: string,
    controller: AbortController,
  ): Promise<string | null> {
    let waitBeforePoll = false;
    while (!controller.signal.aborted) {
      if (waitBeforePoll) {
        await delay(1000, controller.signal);
      }
      waitBeforePoll = true;
      const result = await fetchOperation(operationId, { signal: controller.signal });
      if (controller.signal.aborted) {
        return null;
      }
      if (result.status === 401) {
        expireDuplicateSession();
        return null;
      }
      if (result.status !== 200) {
        setDuplicateProgress(null);
        setDuplicateError(errorMessage(result.body, postgresUnavailable));
        return null;
      }
      const operation = asRecord(result.body.operation);
      const status = operation ? stringField(operation, "status") : "";
      setDuplicateProgress({ id: operationId, status });
      if (status === "succeeded") {
        const succeeded = operation ? asRecord(operation.result) : null;
        const named = succeeded ? stringField(succeeded, "database") : "";
        return named !== "" ? named : submittedName;
      }
      if (status === "failed" || status === "indeterminate" || status === "canceled") {
        const failed = operation ? asRecord(operation.error) : null;
        const message = failed ? stringField(failed, "message") : "";
        setDuplicateProgress(null);
        setDuplicateError(message !== "" ? message : postgresUnavailable);
        return null;
      }
    }
    return null;
  }

  async function handleDuplicate(database: string, owner: string) {
    if (!selected || duplicating || revealing || rotating || creating || truncating || dropping || ticket) {
      return;
    }
    duplicateAbort.current?.abort();
    const controller = new AbortController();
    duplicateAbort.current = controller;
    setDuplicating(true);
    setDuplicateError("");
    try {
      const result = await duplicatePostgresDatabase(selected, database, owner, csrf, {
        signal: controller.signal,
      });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        expireDuplicateSession();
        return;
      }
      if (result.status === 400 || result.status === 403 || result.status === 409) {
        setDuplicateError(
          errorMessage(
            result.body,
            result.status === 403
              ? "This PostgreSQL name is protected"
              : result.status === 409
                ? "A PostgreSQL database with this name already exists"
                : "Invalid database name",
          ),
        );
        return;
      }
      if (result.status === 404) {
        setDuplicateOpen(false);
        setDuplicateError("");
        setTicket(null);
        setRevealError(errorMessage(result.body, "Not found"));
        return;
      }
      if (result.status === 503) {
        const copy = errorMessage(result.body, postgresUnavailable);
        if (copy === isolationRollbackCopy) {
          setDuplicateError(isolationRollbackCopy);
          return;
        }
        setDuplicateOpen(false);
        setDuplicateError("");
        setTicket(null);
        setRevealError(errorMessage(result.body, postgresUnavailable));
        return;
      }
      if (result.status === 202) {
        const operation = asRecord(result.body.operation);
        const operationId = operation ? stringField(operation, "id") : "";
        if (!isOperationId(operationId)) {
          setDuplicateError(errorMessage(result.body, postgresUnavailable));
          return;
        }
        setDuplicateOpen(false);
        setDuplicateError("");
        setTicket(null);
        setTicketRotateWarning(false);
        setDuplicateProgress({ id: operationId, status: operation ? stringField(operation, "status") : "" });
        const createdName = await pollDuplicateOperation(operationId, database, controller);
        if (controller.signal.aborted || createdName === null) {
          return;
        }
        setDuplicateProgress(null);
        setDuplicating(false);
        refreshList();
        openDetails(createdName);
        return;
      }
      if (result.status === 201) {
        const shown = parsePostgresCredential(result.body.credential);
        if (!shown) {
          setDuplicateError(errorMessage(result.body, postgresUnavailable));
          return;
        }
        const createdName =
          typeof result.body.resource?.name === "string" && result.body.resource.name !== ""
            ? result.body.resource.name
            : database;
        setDuplicateOpen(false);
        setDuplicateError("");
        setTicketRotateWarning(false);
        setTicket(shown);
        setPendingSelect(createdName);
        refreshList();
        return;
      }
      setDuplicateError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setDuplicateError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setDuplicating(false);
      }
    }
  }

  async function loadTables(name: string, controller: AbortController) {
    try {
      const result = await fetchPostgresTables(name, { signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 200 && Array.isArray(result.body.tables)) {
        setTables(result.body.tables);
        setTablesTruncated(result.body.truncated === true);
        setTablesError("");
        return;
      }
      setTables(null);
      setTablesTruncated(false);
      setTablesError(errorMessage(result.body, "Tables are unavailable"));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setTables(null);
      setTablesTruncated(false);
      setTablesError("Tables are unavailable");
    } finally {
      if (!controller.signal.aborted) {
        setLoadingTables(false);
      }
    }
  }

  function openTable(table: SelectedTable) {
    if (!selected) {
      return;
    }
    rowsAbort.current?.abort();
    deleteAbort.current?.abort();
    const controller = new AbortController();
    rowsAbort.current = controller;
    setSelectedTable(table);
    setQueryDraft("");
    setAppliedQuery("");
    setRowsQueryError("");
    setRowPage(null);
    setRowsError("");
    setLoadingRows(true);
    setPrimaryKey([]);
    clearRowSelection();
    void loadRows(selected, table.schema, table.name, "", 0, controller);
  }

  function closeTable() {
    rowsAbort.current?.abort();
    deleteAbort.current?.abort();
    clearTruncateSecrets();
    clearDropSecrets();
    clearRowState();
  }

  function applySearch() {
    if (!selected || !selectedTable) {
      return;
    }
    if (queryRuneCount(queryDraft) > maxRowQueryRunes) {
      setRowsQueryError("Query is too long");
      return;
    }
    rowsAbort.current?.abort();
    const controller = new AbortController();
    rowsAbort.current = controller;
    setAppliedQuery(queryDraft);
    setRowsQueryError("");
    setRowPage(null);
    setRowsError("");
    setLoadingRows(true);
    void loadRows(selected, selectedTable.schema, selectedTable.name, queryDraft, 0, controller);
  }

  function pageRows(nextOffset: number) {
    if (!selected || !selectedTable || !rowPage) {
      return;
    }
    rowsAbort.current?.abort();
    const controller = new AbortController();
    rowsAbort.current = controller;
    setRowPage(null);
    setRowsError("");
    setLoadingRows(true);
    void loadRows(selected, selectedTable.schema, selectedTable.name, appliedQuery, nextOffset, controller);
  }

  async function loadRows(
    db: string,
    schema: string,
    table: string,
    q: string,
    offset: number,
    controller: AbortController,
  ) {
    try {
      const pkRequest = fetchPostgresPrimaryKey(db, schema, table, { signal: controller.signal }).catch((err) => {
        if (controller.signal.aborted || isAbortError(err)) {
          throw err;
        }
        return null;
      });
      const [result, pkResult] = await Promise.all([
        fetchPostgresRows(db, schema, table, { q, offset }, { signal: controller.signal }),
        pkRequest,
      ]);
      if (controller.signal.aborted) {
        return;
      }
      if (pkResult && pkResult.status === 200 && Array.isArray(pkResult.body.primary_key)) {
        setPrimaryKey(pkResult.body.primary_key.filter((column): column is string => typeof column === "string"));
      } else {
        setPrimaryKey([]);
      }
      if (result.status === 200 && Array.isArray(result.body.columns) && Array.isArray(result.body.rows)) {
        setRowPage(result.body);
        setRowsError("");
        setRowsQueryError("");
        return;
      }
      setRowPage(null);
      if (result.status === 400 && result.body.error?.fields?.q) {
        setRowsQueryError(errorMessage(result.body, "Query is too long"));
        setRowsError("");
        return;
      }
      if (result.status === 404) {
        setRowsError(errorMessage(result.body, "Not found"));
        return;
      }
      setRowsError(errorMessage(result.body, "PostgreSQL is unavailable"));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setRowPage(null);
      setPrimaryKey([]);
      setRowsError("PostgreSQL is unavailable");
    } finally {
      if (!controller.signal.aborted) {
        setLoadingRows(false);
      }
    }
  }

  function toggleSelectedPk(value: string | number | boolean) {
    const key = pkKey(value);
    setSelectedPks((current) => {
      const next = new Map(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.set(key, value);
      }
      return next;
    });
  }

  async function handleDeleteRows() {
    if (!selected || !selectedTable || deleting || truncating || dropping || ticket !== null || selectedPks.size === 0) {
      return;
    }
    if (deleteConfirmation !== selectedTable.name || deletePassword.length === 0) {
      return;
    }
    const db = selected;
    const schema = selectedTable.schema;
    const table = selectedTable.name;
    const tableConfirmation = deleteConfirmation;
    const ownerPassword = deletePassword;
    const primaryKeyValues = Array.from(selectedPks.values());
    const reloadOffset = rowPage?.offset ?? 0;
    const reloadQuery = appliedQuery;
    deleteAbort.current?.abort();
    const controller = new AbortController();
    deleteAbort.current = controller;
    setDeleting(true);
    setDeleteError("");
    try {
      const result = await deletePostgresRows(
        db,
        schema,
        table,
        csrf,
        tableConfirmation,
        ownerPassword,
        primaryKeyValues,
        { signal: controller.signal },
      );
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setDeleteOpen(false);
        setDeleteError("");
        setDeleteConfirmation("");
        setDeletePassword("");
        setSelectedPks(new Map());
        clearTicket();
        setRowsError(sessionExpired);
        return;
      }
      if (result.status === 403) {
        if (result.body.error?.code === "reauth_required") {
          setDeleteError(errorMessage(result.body, "Owner password is incorrect"));
          setDeletePassword("");
          return;
        }
        setDeleteError(errorMessage(result.body, "Row delete is turned off."));
        return;
      }
      if (result.status === 400) {
        setDeleteError(errorMessage(result.body, "Type the exact table name to confirm deletion"));
        return;
      }
      if (result.status === 404) {
        setDeleteOpen(false);
        setDeleteError("");
        setDeleteConfirmation("");
        setDeletePassword("");
        setSelectedPks(new Map());
        setRowsError(errorMessage(result.body, "Not found"));
        return;
      }
      if (result.status === 503) {
        setDeleteOpen(false);
        setDeleteError("");
        setDeleteConfirmation("");
        setDeletePassword("");
        setSelectedPks(new Map());
        setRowsError(errorMessage(result.body, postgresUnavailable));
        return;
      }
      if (result.status === 200) {
        setDeleteOpen(false);
        setDeleteError("");
        setDeleteConfirmation("");
        setDeletePassword("");
        setSelectedPks(new Map());
        rowsAbort.current?.abort();
        const reload = new AbortController();
        rowsAbort.current = reload;
        setRowPage(null);
        setRowsError("");
        setLoadingRows(true);
        void loadRows(db, schema, table, reloadQuery, reloadOffset, reload);
        return;
      }
      setDeleteError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setDeleteError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setDeleting(false);
      }
    }
  }

  async function handleTruncate() {
    if (!selected || truncating || dropping || revealing || rotating || creating || duplicating || deleting || ticket) {
      return;
    }
    if (truncateConfirmation !== selected || truncatePassword.length === 0) {
      return;
    }
    const db = selected;
    const databaseConfirmation = truncateConfirmation;
    const ownerPassword = truncatePassword;
    const table = selectedTable;
    const reloadOffset = rowPage?.offset ?? 0;
    const reloadQuery = appliedQuery;
    truncateAbort.current?.abort();
    const controller = new AbortController();
    truncateAbort.current = controller;
    setTruncating(true);
    setTruncateError("");
    try {
      const result = await truncatePostgresDatabase(db, databaseConfirmation, ownerPassword, csrf, {
        signal: controller.signal,
      });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setTruncateOpen(false);
        setTruncateConfirmation("");
        setTruncatePassword("");
        setTruncateError(sessionExpired);
        clearTicket();
        return;
      }
      if (result.status === 403) {
        if (result.body.error?.code === "reauth_required") {
          setTruncateError(errorMessage(result.body, "Owner password is incorrect"));
          setTruncatePassword("");
          return;
        }
        setTruncateError(errorMessage(result.body, "Truncate is turned off."));
        return;
      }
      if (result.status === 409) {
        setTruncateError(errorMessage(result.body, postgresUnavailable));
        return;
      }
      if (result.status === 400) {
        setTruncateError(errorMessage(result.body, "Type the exact database name to confirm truncate"));
        return;
      }
      if (result.status === 404) {
        setTruncateOpen(false);
        setTruncateConfirmation("");
        setTruncatePassword("");
        setTruncateError(errorMessage(result.body, "Not found"));
        return;
      }
      if (result.status === 503) {
        setTruncateOpen(false);
        setTruncateConfirmation("");
        setTruncatePassword("");
        setTruncateError(errorMessage(result.body, postgresUnavailable));
        return;
      }
      if (result.status === 200) {
        setTruncateOpen(false);
        setTruncateConfirmation("");
        setTruncatePassword("");
        setTruncateError("");
        setSelectedPks(new Map());
        const tablesController = selectionAbort.current ?? new AbortController();
        selectionAbort.current = tablesController;
        setLoadingTables(true);
        void loadTables(db, tablesController);
        if (table) {
          rowsAbort.current?.abort();
          const rowsReload = new AbortController();
          rowsAbort.current = rowsReload;
          setRowPage(null);
          setRowsError("");
          setLoadingRows(true);
          void loadRows(db, table.schema, table.name, reloadQuery, reloadOffset, rowsReload);
        }
        return;
      }
      setTruncateError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setTruncateError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setTruncating(false);
      }
    }
  }

  async function handleDrop() {
    if (!selected || dropping || truncating || revealing || rotating || creating || duplicating || deleting || ticket) {
      return;
    }
    if (dropConfirmation !== selected || dropPassword.length === 0) {
      return;
    }
    const db = selected;
    const databaseConfirmation = dropConfirmation;
    const ownerPassword = dropPassword;
    dropAbort.current?.abort();
    const controller = new AbortController();
    dropAbort.current = controller;
    setDropping(true);
    setDropError("");
    try {
      const result = await dropPostgresDatabase(db, databaseConfirmation, ownerPassword, csrf, {
        signal: controller.signal,
      });
      if (controller.signal.aborted) {
        return;
      }
      if (result.status === 401) {
        setDropOpen(false);
        setDropConfirmation("");
        setDropPassword("");
        setDropError(sessionExpired);
        clearTicket();
        return;
      }
      if (result.status === 403) {
        if (result.body.error?.code === "reauth_required") {
          setDropError(errorMessage(result.body, "Owner password is incorrect"));
          setDropPassword("");
          return;
        }
        setDropError(errorMessage(result.body, "Drop is turned off."));
        return;
      }
      if (result.status === 409) {
        setDropError(errorMessage(result.body, postgresUnavailable));
        return;
      }
      if (result.status === 400) {
        setDropError(errorMessage(result.body, "Type the exact database name to confirm drop"));
        return;
      }
      if (result.status === 404) {
        setDropOpen(false);
        setDropConfirmation("");
        setDropPassword("");
        setDropError(errorMessage(result.body, "Not found"));
        return;
      }
      if (result.status === 503) {
        setDropOpen(false);
        setDropConfirmation("");
        setDropPassword("");
        setDropError(errorMessage(result.body, postgresUnavailable));
        return;
      }
      if (result.status === 200) {
        setDropOpen(false);
        setDropConfirmation("");
        setDropPassword("");
        setDropError("");
        selectionAbort.current?.abort();
        rowsAbort.current?.abort();
        revealAbort.current?.abort();
        rotateAbort.current?.abort();
        duplicateAbort.current?.abort();
        deleteAbort.current?.abort();
        truncateAbort.current?.abort();
        setSelected(null);
        setDetails(null);
        setDetailsError("");
        setLoadingDetails(false);
        setConnection(null);
        setConnectionError("");
        setLoadingConnection(false);
        setTables(null);
        setTablesError("");
        setTablesTruncated(false);
        setLoadingTables(false);
        clearTicket();
        clearTruncateSecrets();
        clearRowState();
        refreshList();
        return;
      }
      setDropError(errorMessage(result.body, postgresUnavailable));
    } catch (err) {
      if (controller.signal.aborted || isAbortError(err)) {
        return;
      }
      setDropError(postgresUnavailable);
    } finally {
      if (!controller.signal.aborted) {
        setDropping(false);
      }
    }
  }

  const showReveal =
    !loadingDetails &&
    !loadingConnection &&
    connectionError === "" &&
    connection?.savedCredentialStatus === "present";
  const showRotate = !loadingDetails && rotationEligible(details);
  const showDuplicate = !loadingDetails && rotationEligible(details);
  const showTruncate = !loadingDetails && details !== null;
  const showDrop = !loadingDetails && details !== null;
  const showCreate = items !== null && listError === "";
  const mutationBusy = creating || revealing || rotating || duplicating || truncating || dropping || ticket !== null;
  const createDisabled = mutationBusy;
  const truncateDisabled = mutationBusy || deleting;
  const dropDisabled = mutationBusy || deleting;

  return (
    <article>
      <header className="page-header">
        <div className="page-header-row">
          <h1>Databases</h1>
          {showCreate ? (
            <button
              type="button"
              className="primary-button"
              disabled={createDisabled}
              onClick={() => {
                setCreateError("");
                setCreateOpen(true);
              }}
            >
              Create database
            </button>
          ) : null}
        </div>
        <p>Manageable project databases only. Passwords are not revealed.</p>
      </header>
      {duplicateProgress ? (
        <p className="muted-copy" role="status">
          Duplicating database. Operation{" "}
          <span className="identifier">{displayText(duplicateProgress.id)}</span>.
        </p>
      ) : null}
      {!duplicateOpen && duplicateError ? (
        <p className="form-warning" role="alert">
          {duplicateError}
        </p>
      ) : null}
      {ticket ? (
        <CredentialTicket
          kind="postgres"
          credential={ticket}
          rotateWarning={ticketRotateWarning}
          onDismiss={dismissTicket}
        />
      ) : null}
      {createOpen ? (
        <CreateDatabaseForm
          error={createError}
          submitting={creating}
          onCancel={() => {
            if (creating) {
              return;
            }
            setCreateOpen(false);
            setCreateError("");
          }}
          onSubmit={(database, owner) => void handleCreate(database, owner)}
        />
      ) : null}
      {rotateOpen && selected ? (
        <RotatePasswordDialog
          database={selected}
          error={rotateError}
          submitting={rotating}
          onCancel={() => {
            if (rotating) {
              return;
            }
            setRotateOpen(false);
            setRotateError("");
          }}
          onConfirm={(confirmation) => void handleRotate(confirmation)}
        />
      ) : null}
      {duplicateOpen && selected ? (
        <DuplicateDatabaseForm
          source={selected}
          connectionCount={details?.connection_count ?? 0}
          error={duplicateError}
          submitting={duplicating}
          onCancel={() => {
            if (duplicating) {
              return;
            }
            setDuplicateOpen(false);
            setDuplicateError("");
          }}
          onSubmit={(database, owner) => void handleDuplicate(database, owner)}
        />
      ) : null}
      {deleteOpen && selectedTable ? (
        <DeleteSelectedRowsDialog
          schema={selectedTable.schema}
          table={selectedTable.name}
          selectedCount={selectedPks.size}
          confirmation={deleteConfirmation}
          password={deletePassword}
          error={deleteError}
          submitting={deleting}
          onConfirmationChange={setDeleteConfirmation}
          onPasswordChange={setDeletePassword}
          onCancel={() => {
            if (deleting) {
              return;
            }
            setDeleteOpen(false);
            setDeleteError("");
            setDeleteConfirmation("");
            setDeletePassword("");
          }}
          onConfirm={() => void handleDeleteRows()}
        />
      ) : null}
      {truncateOpen && selected ? (
        <TruncateProjectDataDialog
          database={selected}
          confirmation={truncateConfirmation}
          password={truncatePassword}
          error={truncateError}
          submitting={truncating}
          onConfirmationChange={setTruncateConfirmation}
          onPasswordChange={setTruncatePassword}
          onCancel={() => {
            if (truncating) {
              return;
            }
            setTruncateOpen(false);
            setTruncateError("");
            setTruncateConfirmation("");
            setTruncatePassword("");
          }}
          onConfirm={() => void handleTruncate()}
        />
      ) : null}
      {dropOpen && selected ? (
        <DropDatabaseDialog
          database={selected}
          confirmation={dropConfirmation}
          password={dropPassword}
          error={dropError}
          submitting={dropping}
          onConfirmationChange={setDropConfirmation}
          onPasswordChange={setDropPassword}
          onCancel={() => {
            if (dropping) {
              return;
            }
            setDropOpen(false);
            setDropError("");
            setDropConfirmation("");
            setDropPassword("");
          }}
          onConfirm={() => void handleDrop()}
        />
      ) : null}
      {listError ? (
        <p className="form-warning" role="alert">
          {listError}
        </p>
      ) : items === null ? (
        <p className="muted-copy">Loading databases.</p>
      ) : items.length === 0 ? (
        <p className="muted-copy">No manageable project databases.</p>
      ) : (
        <ul className="ledger-list">
          {items.map((item) => {
            const name = item.name ?? "";
            if (!name) {
              return null;
            }
            return (
              <li key={name}>
                <button
                  type="button"
                  className={selected === name ? "ledger-item ledger-item-active" : "ledger-item"}
                  aria-current={selected === name ? "true" : undefined}
                  onClick={() => void openDetails(name)}
                >
                  <span className="identifier">{name}</span>
                  <span className="muted-copy identifier">{item.owner ?? ""}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {truncated ? <p className="form-warning">List truncated at 500 databases.</p> : null}
      {selected ? (
        <section
          className="detail-panel"
          aria-label="Database details"
          aria-busy={loadingDetails || loadingConnection || loadingTables || loadingRows}
        >
          <h2 className="identifier">{selected}</h2>
          {loadingDetails ? (
            <p className="muted-copy" role="status">
              Loading details.
            </p>
          ) : null}
          {detailsError ? (
            <p className="form-warning" role="alert">
              {detailsError}
            </p>
          ) : null}
          {details ? <DetailsFacts details={details} /> : null}
          {loadingConnection ? (
            <p className="muted-copy" role="status">
              Loading connection.
            </p>
          ) : null}
          {connectionError ? (
            <p className="form-warning" role="alert">
              {connectionError}
            </p>
          ) : null}
          {connection ? <ConnectionFacts urls={connection} /> : null}
          {revealError ? (
            <p className="form-warning" role="alert">
              {revealError}
            </p>
          ) : null}
          {showReveal || showRotate || showDuplicate ? (
            <div className="form-actions">
              {showReveal ? (
                <button
                  type="button"
                  className="text-button"
                  disabled={mutationBusy}
                  onClick={() => void handleReveal()}
                >
                  Reveal
                </button>
              ) : null}
              {showRotate ? (
                <button
                  type="button"
                  className="text-button"
                  disabled={mutationBusy}
                  onClick={() => {
                    setRotateError("");
                    setRotateOpen(true);
                  }}
                >
                  Rotate
                </button>
              ) : null}
              {showDuplicate ? (
                <button
                  type="button"
                  className="text-button"
                  disabled={mutationBusy}
                  onClick={() => {
                    setDuplicateError("");
                    setDuplicateOpen(true);
                  }}
                >
                  Duplicate
                </button>
              ) : null}
            </div>
          ) : null}
          {showTruncate || showDrop ? (
            <div className="form-actions">
              {showTruncate ? (
                <button
                  type="button"
                  className="danger-button"
                  disabled={truncateDisabled}
                  onClick={() => {
                    setTruncateError("");
                    setTruncateOpen(true);
                  }}
                >
                  Truncate
                </button>
              ) : null}
              {showDrop ? (
                <button
                  type="button"
                  className="danger-button"
                  disabled={dropDisabled}
                  onClick={() => {
                    setDropError("");
                    setDropOpen(true);
                  }}
                >
                  Drop
                </button>
              ) : null}
            </div>
          ) : null}
          {!truncateOpen && truncateError ? (
            <p className="form-warning" role="alert">
              {truncateError}
            </p>
          ) : null}
          {!dropOpen && dropError ? (
            <p className="form-warning" role="alert">
              {dropError}
            </p>
          ) : null}
          <h3>Tables</h3>
          {loadingTables ? (
            <p className="muted-copy" role="status">
              Loading tables.
            </p>
          ) : null}
          {tablesError ? (
            <p className="form-warning" role="alert">
              {tablesError}
            </p>
          ) : null}
          {tablesTruncated ? <p className="form-warning">Table list truncated at 500 tables.</p> : null}
          {tables && tables.length === 0 && !tablesError ? <p className="muted-copy">No tables.</p> : null}
          {tables && tables.length > 0 ? (
            <TableNameList
              tables={tables}
              selected={selectedTable}
              onSelect={openTable}
              onBack={closeTable}
              rows={
                selectedTable ? (
                  <RowsPanel
                    regionRef={rowsRegionRef}
                    table={selectedTable}
                    page={rowPage}
                    error={rowsError}
                    queryError={rowsQueryError}
                    loading={loadingRows}
                    queryDraft={queryDraft}
                    onQueryDraftChange={setQueryDraft}
                    onSearch={applySearch}
                    onPrevious={() => pageRows(Math.max(0, (rowPage?.offset ?? 0) - (rowPage?.limit ?? 0)))}
                    onNext={() => pageRows((rowPage?.offset ?? 0) + (rowPage?.limit ?? 0))}
                    primaryKey={primaryKey}
                    selectedPks={selectedPks}
                    ticketOpen={ticket !== null}
                    deleting={deleting}
                    onToggleSelected={toggleSelectedPk}
                    onDeleteSelected={() => {
                      setDeleteError("");
                      setDeleteOpen(true);
                    }}
                  />
                ) : null
              }
            />
          ) : null}
        </section>
      ) : null}
    </article>
  );
}

function TableNameList({
  tables,
  selected,
  onSelect,
  onBack,
  rows,
}: {
  tables: TableItem[];
  selected: SelectedTable | null;
  onSelect: (table: SelectedTable) => void;
  onBack: () => void;
  rows: ReactNode;
}) {
  return (
    <>
      {selected ? (
        <p>
          <button type="button" className="text-button" onClick={onBack}>
            Back to tables
          </button>
        </p>
      ) : null}
      <ul className={selected ? "table-name-list table-name-list-inspecting" : "table-name-list"}>
        {tables.flatMap((table) => {
          const schema = table.schema ?? "";
          const name = table.name ?? "";
          if (!schema || !name) {
            return [];
          }
          const active = selected?.schema === schema && selected?.name === name;
          return [
            <li key={`${schema}.${name}`} className={active ? "is-selected" : undefined}>
              <button
                type="button"
                className={active ? "table-name-item table-name-item-active" : "table-name-item"}
                aria-label={`Schema ${schema} Table ${name}`}
                aria-current={active ? "true" : undefined}
                onClick={() => onSelect({ schema, name })}
              >
                <span>
                  <span className="muted-copy">Schema </span>
                  <span className="identifier">{schema}</span>
                </span>
                <span>
                  <span className="muted-copy">Table </span>
                  <span className="identifier">{name}</span>
                </span>
              </button>
            </li>,
            active ? (
              <li key={`${schema}.${name}.rows`} className="table-rows-slot is-selected">
                {rows}
              </li>
            ) : null,
          ];
        })}
      </ul>
    </>
  );
}

function RowsPanel({
  regionRef,
  table,
  page,
  error,
  queryError,
  loading,
  queryDraft,
  onQueryDraftChange,
  onSearch,
  onPrevious,
  onNext,
  primaryKey,
  selectedPks,
  ticketOpen,
  deleting,
  onToggleSelected,
  onDeleteSelected,
}: {
  regionRef: RefObject<HTMLElement | null>;
  table: SelectedTable;
  page: RowPage | null;
  error: string;
  queryError: string;
  loading: boolean;
  queryDraft: string;
  onQueryDraftChange: (value: string) => void;
  onSearch: () => void;
  onPrevious: () => void;
  onNext: () => void;
  primaryKey: string[];
  selectedPks: Map<string, string | number | boolean>;
  ticketOpen: boolean;
  deleting: boolean;
  onToggleSelected: (value: string | number | boolean) => void;
  onDeleteSelected: () => void;
}) {
  const columns = page?.columns ?? [];
  const rows = page?.rows ?? [];
  const offset = page?.offset ?? 0;
  const total = page?.total ?? 0;
  const previousDisabled = page == null || offset === 0;
  const nextDisabled = page == null || offset + rows.length >= total;
  const range = rows.length > 0 ? `${offset + 1}–${offset + rows.length} of ${total}` : "";
  const pkColumn = primaryKey.length === 1 ? primaryKey[0] : "";
  const showDelete = !loading && pkColumn !== "";
  const deleteDisabled = deleting || ticketOpen || selectedPks.size === 0;

  return (
    <section
      ref={regionRef}
      className="rows-region"
      aria-label={`Rows for ${table.schema}.${table.name}`}
      aria-busy={loading}
    >
      <h3>
        <span className="identifier">{table.schema}</span>
        <span className="muted-copy">.</span>
        <span className="identifier">{table.name}</span>
      </h3>
      <form
        className="row-search"
        onSubmit={(event) => {
          event.preventDefault();
          onSearch();
        }}
      >
        <div className="row-search-field">
          <label htmlFor="row-query">Search rows</label>
          <input
            id="row-query"
            autoComplete="off"
            value={queryDraft}
            onChange={(event) => onQueryDraftChange(event.target.value)}
            aria-invalid={queryError ? true : undefined}
            aria-describedby={queryError ? "row-query-hint row-query-error" : "row-query-hint"}
          />
          <p id="row-query-hint" className="muted-copy">
            Maximum 128 code points. Apply to search.
          </p>
        </div>
        <button type="submit" className="text-button">
          Apply
        </button>
      </form>
      {queryError ? (
        <p id="row-query-error" className="form-error" role="alert">
          {queryError}
        </p>
      ) : null}
      {loading ? (
        <p className="muted-copy" role="status">
          Loading rows.
        </p>
      ) : null}
      {error ? (
        <p className="form-warning" role="alert">
          {error}
        </p>
      ) : null}
      {showDelete ? (
        <div className="form-actions">
          <button
            type="button"
            className="danger-button"
            disabled={deleteDisabled}
            onClick={onDeleteSelected}
          >
            Delete selected
          </button>
        </div>
      ) : null}
      {page && rows.length === 0 && !error ? (
        <p className="muted-copy" role="status">
          No rows.
        </p>
      ) : null}
      {page && rows.length > 0 && !error ? (
        <RowGrid
          schema={table.schema}
          table={table.name}
          columns={columns}
          rows={rows}
          pkColumn={pkColumn}
          selectedPks={selectedPks}
          onToggleSelected={onToggleSelected}
        />
      ) : null}
      {range ? (
        <p className="muted-copy" role="status">
          {range}
        </p>
      ) : null}
      {page && !error ? (
        <div className="row-pager">
          <button type="button" className="text-button" disabled={previousDisabled} onClick={onPrevious}>
            Previous
          </button>
          <button type="button" className="text-button" disabled={nextDisabled} onClick={onNext}>
            Next
          </button>
        </div>
      ) : null}
    </section>
  );
}

function RowGrid({
  schema,
  table,
  columns,
  rows,
  pkColumn,
  selectedPks,
  onToggleSelected,
}: {
  schema: string;
  table: string;
  columns: string[];
  rows: Array<Record<string, unknown>>;
  pkColumn: string;
  selectedPks: Map<string, string | number | boolean>;
  onToggleSelected: (value: string | number | boolean) => void;
}) {
  const showSelect = pkColumn !== "";
  return (
    <div className="row-grid-wrap">
      <table className="row-grid">
        <caption className="visually-hidden">
          Rows for {schema}.{table}
        </caption>
        <thead>
          <tr>
            {showSelect ? (
              <th className="row-select" scope="col">
                <span className="visually-hidden">Select</span>
              </th>
            ) : null}
            {columns.map((column) => (
              <th key={column} scope="col">
                <span className="identifier">{column}</span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => {
            const scalar = showSelect ? pkScalar(row[pkColumn]) : null;
            const key = scalar === null ? String(index) : pkKey(scalar);
            return (
              <tr key={key}>
                {showSelect ? (
                  <td className="row-select">
                    {scalar === null ? null : (
                      <label className="row-select-label">
                        <input
                          type="checkbox"
                          checked={selectedPks.has(pkKey(scalar))}
                          onChange={() => onToggleSelected(scalar)}
                          aria-label={`Select row ${displayText(String(scalar))}`}
                        />
                      </label>
                    )}
                  </td>
                ) : null}
                {columns.map((column) => {
                  const cell = formatCell(row[column]);
                  return (
                    <td key={column} className={cell.nullish ? "muted-copy" : "identifier"}>
                      {cell.text}
                    </td>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function formatCell(value: unknown): { text: string; nullish: boolean } {
  if (value === null || value === undefined) {
    return { text: "Null", nullish: true };
  }
  if (typeof value === "boolean" || typeof value === "number" || typeof value === "string") {
    return { text: String(value), nullish: false };
  }
  try {
    return { text: JSON.stringify(value), nullish: false };
  } catch {
    return { text: "Null", nullish: true };
  }
}

function ConnectionFacts({ urls }: { urls: ConnectionUrls }) {
  const directUrl = urls.maskedDirectUrl;
  const pooledUrl = urls.maskedPooledUrl;
  if (!directUrl && !pooledUrl) {
    return null;
  }
  return (
    <dl className="fact-list">
      {directUrl ? (
        <div>
          <dt>Direct URL</dt>
          <dd className="bidi-isolate identifier">{displayText(directUrl)}</dd>
          <button type="button" className="text-button" onClick={() => void copyText(directUrl)}>
            Copy Direct URL
          </button>
        </div>
      ) : null}
      {pooledUrl ? (
        <div>
          <dt>Pooled URL</dt>
          <dd className="bidi-isolate identifier">{displayText(pooledUrl)}</dd>
          <button type="button" className="text-button" onClick={() => void copyText(pooledUrl)}>
            Copy Pooled URL
          </button>
        </div>
      ) : null}
    </dl>
  );
}

function DetailsFacts({ details }: { details: DatabaseDetails }) {
  return (
    <dl className="fact-list">
      <Fact label="Owner" value={details.owner ?? "—"} kind="identifier" />
      <Fact label="Size" value={details.size ?? "—"} kind="metric" />
      <Fact label="Collation" value={details.collation ?? "—"} kind="identifier" />
      <Fact label="Ctype" value={details.ctype ?? "—"} kind="identifier" />
      <Fact
        label="Connections"
        value={details.connection_count == null ? "—" : String(details.connection_count)}
        kind="metric"
      />
      <Fact label="PUBLIC CONNECT" value={yesNo(details.security?.public_can_connect)} />
      <Fact label="Owner is superuser" value={yesNo(details.security?.owner_is_superuser)} />
      <Fact label="Owner can log in" value={yesNo(details.security?.owner_can_login)} />
      <Fact label="Owner can create databases" value={yesNo(details.security?.owner_createdb)} />
      <Fact label="Owner can create roles" value={yesNo(details.security?.owner_createrole)} />
      <Fact label="Owner replication" value={yesNo(details.security?.owner_replication)} />
      <Fact label="Saved credential" value={savedCredentialCopy(details.saved_credential?.status)} />
    </dl>
  );
}

function Fact({
  label,
  value,
  kind,
}: {
  label: string;
  value: string;
  kind?: "identifier" | "metric";
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={kind === "identifier" ? "identifier" : kind === "metric" ? "metric" : undefined}>{value}</dd>
    </div>
  );
}

function savedCredentialCopy(status: string | undefined): string {
  if (status === "present") {
    return "Saved";
  }
  if (status === "missing") {
    return "Not saved";
  }
  return "Not available";
}

function yesNo(value: boolean | undefined): string {
  if (value === true) {
    return "Yes";
  }
  if (value === false) {
    return "No";
  }
  return "—";
}
