import { createContext, useContext, useState } from "react";
import { BrowserRouter, NavLink, Navigate, Route, Routes } from "react-router-dom";
import { BuyerPage } from "./pages/BuyerPage";
import { SalesPage } from "./pages/SalesPage";
import {
  initialWorkspaceId,
  storeWorkspaceId,
  workspaces,
} from "./workspaces";

const WorkspaceContext = createContext<string | null>(null);

export function useWorkspaceId(): string {
  const id = useContext(WorkspaceContext);
  if (!id) {
    throw new Error("Workspace missing");
  }
  return id;
}

export function App() {
  const [workspaceId, setWorkspaceId] = useState(initialWorkspaceId);
  const workspace =
    workspaces.find((item) => item.id === workspaceId) ?? workspaces[0];

  function choose(id: string) {
    setWorkspaceId(id);
    storeWorkspaceId(id);
  }

  return (
    <WorkspaceContext.Provider value={workspace.id}>
      <BrowserRouter>
        <div className="app">
          <header className="top">
            <div>
              <p className="eyebrow">Beauty brands</p>
              <h1>Order requests</h1>
              <p className="workspace-name">{workspace.name}</p>
            </div>
            <nav className="nav">
              <NavLink to="/" end>
                Buyer
              </NavLink>
              <NavLink to="/sales">Sales</NavLink>
            </nav>
            <label className="workspace">
              <span>Workspace</span>
              <select
                value={workspace.id}
                onChange={(event) => choose(event.target.value)}
              >
                {workspaces.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          </header>
          <main>
            <Routes>
              <Route path="/" element={<BuyerPage key={workspace.id} />} />
              <Route path="/sales" element={<SalesPage key={workspace.id} />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </main>
        </div>
      </BrowserRouter>
    </WorkspaceContext.Provider>
  );
}
