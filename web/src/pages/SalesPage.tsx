import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { formatMoney } from "../format";
import type { DraftOrder, OrderRequest, OrderRequestPage } from "../types";
import { useWorkspaceId } from "../App";

const filters = ["new", "accepted", "rejected", "all"] as const;
type Filter = (typeof filters)[number];

const limit = 20;

export function SalesPage() {
  const workspaceId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [filter, setFilter] = useState<Filter>("new");
  const [offset, setOffset] = useState(0);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [notice, setNotice] = useState("");

  const list = useQuery({
    queryKey: ["order-requests", workspaceId, filter, offset],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: String(limit),
        offset: String(offset),
      });
      if (filter !== "all") {
        params.set("status", filter);
      }
      return api<OrderRequestPage>(
        workspaceId,
        `/api/order-requests?${params.toString()}`,
      );
    },
  });

  const detail = useQuery({
    queryKey: ["order-request", workspaceId, selectedId],
    enabled: Boolean(selectedId),
    queryFn: () =>
      api<OrderRequest>(workspaceId, `/api/order-requests/${selectedId}`),
  });

  function refresh() {
    void queryClient.invalidateQueries({ queryKey: ["order-requests", workspaceId] });
    void queryClient.invalidateQueries({ queryKey: ["order-request", workspaceId] });
    void queryClient.invalidateQueries({ queryKey: ["products", workspaceId] });
  }

  const accept = useMutation({
    mutationFn: (id: string) =>
      api<DraftOrder>(workspaceId, `/api/order-requests/${id}/accept`, {
        method: "POST",
      }),
    onSuccess: () => {
      setNotice("Draft order created. Stock is reserved.");
      setSelectedId(null);
      refresh();
    },
  });

  const reject = useMutation({
    mutationFn: (id: string) =>
      api<{ id: string; status: string }>(
        workspaceId,
        `/api/order-requests/${id}/reject`,
        { method: "POST" },
      ),
    onSuccess: () => {
      setNotice("Request rejected. Stock was not changed.");
      setSelectedId(null);
      refresh();
    },
  });

  const page = list.data;
  const busy = accept.isPending || reject.isPending;

  return (
    <section className="panel">
      <p className="eyebrow">Sales</p>
      <h2>Review requests</h2>
      <div className="filters" role="tablist" aria-label="Status">
        {filters.map((item) => (
          <button
            key={item}
            type="button"
            role="tab"
            aria-selected={filter === item}
            className={filter === item ? "tab active" : "tab"}
            onClick={() => {
              setFilter(item);
              setOffset(0);
              setSelectedId(null);
              setNotice("");
            }}
          >
            {item.charAt(0).toUpperCase() + item.slice(1)}
          </button>
        ))}
      </div>

      {notice && (
        <p className="banner ok" role="status">
          {notice}
        </p>
      )}
      {list.isLoading && <p className="status">Loading requests…</p>}
      {list.isError && (
        <div className="banner error" role="alert">
          <p>{list.error.message}</p>
          <button type="button" onClick={() => void list.refetch()}>
            Try again
          </button>
        </div>
      )}
      {page && page.items.length === 0 && (
        <p className="status">No {filter === "all" ? "" : `${filter} `}requests.</p>
      )}

      {page && page.items.length > 0 && (
        <div className="sales">
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Buyer</th>
                  <th>Status</th>
                  <th>Total</th>
                </tr>
              </thead>
              <tbody>
                {page.items.map((item) => (
                  <tr
                    key={item.id}
                    className={item.id === selectedId ? "selected" : "clickable"}
                    onClick={() => {
                      setSelectedId(item.id);
                      setNotice("");
                      accept.reset();
                      reject.reset();
                    }}
                  >
                    <td>
                      <button type="button" className="row-button">
                        {item.buyerName}
                        <span>{item.buyerEmail}</span>
                      </button>
                    </td>
                    <td>
                      <span className={`pill pill-${item.status}`}>{item.status}</span>
                    </td>
                    <td>{formatMoney(item.totalMinor, item.currency)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <aside className="detail">
            {!selectedId && <p className="status">Select a request.</p>}
            {selectedId && detail.isLoading && <p className="status">Loading request…</p>}
            {selectedId && detail.isError && (
              <div className="banner error" role="alert">
                <p>{detail.error.message}</p>
                <button type="button" onClick={() => void detail.refetch()}>
                  Try again
                </button>
              </div>
            )}
            {detail.data && (
              <>
                <h3>{detail.data.buyerName}</h3>
                <p className="muted">{detail.data.buyerEmail}</p>
                <p>
                  <span className={`pill pill-${detail.data.status}`}>{detail.data.status}</span>
                </p>
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Product</th>
                        <th>Qty</th>
                        <th>Line</th>
                      </tr>
                    </thead>
                    <tbody>
                      {detail.data.lines.map((line) => (
                        <tr key={line.productId}>
                          <td>{line.productName}</td>
                          <td>{line.quantity}</td>
                          <td>{formatMoney(line.lineTotalMinor, line.currency)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <p className="total">
                  Total {formatMoney(detail.data.totalMinor, detail.data.currency)}
                </p>
                {detail.data.status === "new" && (
                  <div className="actions">
                    <button
                      type="button"
                      className="primary"
                      disabled={busy}
                      onClick={() => accept.mutate(detail.data.id)}
                    >
                      {accept.isPending ? "Accepting…" : "Accept"}
                    </button>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => reject.mutate(detail.data.id)}
                    >
                      {reject.isPending ? "Rejecting…" : "Reject"}
                    </button>
                  </div>
                )}
                {(accept.isError || reject.isError) && (
                  <p className="banner error" role="alert">
                    {accept.error?.message || reject.error?.message}
                  </p>
                )}
              </>
            )}
          </aside>
        </div>
      )}

      {page && page.total > limit && (
        <div className="pager">
          <button
            type="button"
            disabled={offset === 0}
            onClick={() => setOffset((current) => Math.max(0, current - limit))}
          >
            Previous
          </button>
          <span>
            {offset + 1}–{Math.min(offset + page.items.length, page.total)} of {page.total}
          </span>
          <button
            type="button"
            disabled={offset + limit >= page.total}
            onClick={() => setOffset((current) => current + limit)}
          >
            Next
          </button>
        </div>
      )}
    </section>
  );
}
