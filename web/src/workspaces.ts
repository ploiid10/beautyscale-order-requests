export type Workspace = {
  id: string;
  name: string;
};

export const workspaces: Workspace[] = [
  {
    id: "8f1c0b2e-6a4d-4c1e-9b7a-1d2e3f4a5b6c",
    name: "Lumen Atelier",
  },
  {
    id: "3c9e1a70-2b54-4f8d-a6e1-7b8c9d0e1f2a",
    name: "Northline Beauty",
  },
];

const storageKey = "workspaceId";

export function initialWorkspaceId(): string {
  const saved = localStorage.getItem(storageKey);
  if (saved && workspaces.some((workspace) => workspace.id === saved)) {
    return saved;
  }
  return workspaces[0].id;
}

export function storeWorkspaceId(id: string) {
  localStorage.setItem(storageKey, id);
}
