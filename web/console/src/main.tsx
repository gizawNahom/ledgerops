// main -- Vite app entry point. Composition root: wires the KeyStorage and
// ApiClient adapters and mounts ConsoleApp into the DOM (DESIGN SA-D4).
// The only side-effecting module outside the adapters themselves.
import React from "react";
import ReactDOM from "react-dom/client";
import { ConsoleApp } from "./components/ConsoleApp";
import { createKeyStorage } from "./keyStorage";
import { createApiClient } from "./apiClient";
import "./index.css";

const keyStorage = createKeyStorage();
const apiClient = createApiClient({ keyStorage });

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("ledgerops console: #root element not found in index.html");
}

ReactDOM.createRoot(rootElement).render(
  <React.StrictMode>
    <ConsoleApp keyStorage={keyStorage} apiClient={apiClient} />
  </React.StrictMode>,
);
