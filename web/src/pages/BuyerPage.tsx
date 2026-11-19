import { FormEvent, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { formatMoney } from "../format";
import type { OrderRequest, Product } from "../types";
import { useWorkspaceId } from "../App";

export function BuyerPage() {
  const workspaceId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [buyerName, setBuyerName] = useState("");
  const [buyerEmail, setBuyerEmail] = useState("");
  const [quantities, setQuantities] = useState<Record<string, string>>({});
  const [confirmation, setConfirmation] = useState<OrderRequest | null>(null);
  const [formError, setFormError] = useState("");

  const products = useQuery({
    queryKey: ["products", workspaceId],
    queryFn: () => api<Product[]>(workspaceId, "/api/products"),
  });

  const submit = useMutation({
    mutationFn: (body: {
      buyerName: string;
      buyerEmail: string;
      lines: { productId: string; quantity: number }[];
    }) =>
      api<OrderRequest>(workspaceId, "/api/order-requests", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: (created) => {
      setConfirmation(created);
      setFormError("");
      setQuantities({});
      void queryClient.invalidateQueries({ queryKey: ["products", workspaceId] });
    },
  });

  const selection = useMemo(() => {
    const catalog = products.data ?? [];
    const lines: { product: Product; quantity: number }[] = [];
    let error = "";
    for (const product of catalog) {
      const raw = quantities[product.id]?.trim() ?? "";
      if (!raw) {
        continue;
      }
      const quantity = Number(raw);
      if (!Number.isInteger(quantity) || quantity < 0 || quantity > 10000) {
        error = "Quantities are whole numbers from 0 to 10000.";
        continue;
      }
      if (quantity > 0) {
        lines.push({ product, quantity });
      }
    }
    const currencies = new Set(lines.map((line) => line.product.currency));
    if (currencies.size > 1) {
      error = "A request can only use one currency.";
    }
    const currency = lines[0]?.product.currency ?? "";
    const totalMinor = lines.reduce(
      (sum, line) => sum + line.product.priceMinor * line.quantity,
      0,
    );
    return { lines, error, currency, totalMinor };
  }, [products.data, quantities]);

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    setFormError("");
    if (!buyerName.trim() || !buyerEmail.trim()) {
      setFormError("Name and email are required.");
      return;
    }
    if (selection.error) {
      setFormError(selection.error);
      return;
    }
    if (selection.lines.length === 0) {
      setFormError("Add a quantity for at least one product.");
      return;
    }
    submit.mutate({
      buyerName: buyerName.trim(),
      buyerEmail: buyerEmail.trim(),
      lines: selection.lines.map((line) => ({
        productId: line.product.id,
        quantity: line.quantity,
      })),
    });
  }

  if (confirmation) {
    return (
      <section className="panel">
        <p className="eyebrow">Request sent</p>
        <h2>Thanks, {confirmation.buyerName}.</h2>
        <p className="lede">
          Sales will review this request. Nothing is reserved until they accept it.
        </p>
        <dl className="meta">
          <div>
            <dt>Request</dt>
            <dd>{confirmation.id}</dd>
          </div>
          <div>
            <dt>Status</dt>
            <dd>
              <span className={`pill pill-${confirmation.status}`}>{confirmation.status}</span>
            </dd>
          </div>
          <div>
            <dt>Total</dt>
            <dd>{formatMoney(confirmation.totalMinor, confirmation.currency)}</dd>
          </div>
        </dl>
        <LineTable lines={confirmation.lines} />
        <button
          type="button"
          className="primary"
          onClick={() => {
            setConfirmation(null);
            setBuyerName("");
            setBuyerEmail("");
          }}
        >
          New request
        </button>
      </section>
    );
  }

  return (
    <section className="panel">
      <p className="eyebrow">Buyer</p>
      <h2>Ask for a shipment</h2>
      <p className="lede">
        Choose quantities. The brand reviews the request before any stock is reserved.
      </p>

      {products.isLoading && <p className="status">Loading products…</p>}
      {products.isError && (
        <div className="banner error" role="alert">
          <p>{products.error.message}</p>
          <button type="button" onClick={() => void products.refetch()}>
            Try again
          </button>
        </div>
      )}
      {products.data && products.data.length === 0 && (
        <p className="status">This workspace has no products yet.</p>
      )}

      {products.data && products.data.length > 0 && (
        <form onSubmit={onSubmit} className="stack">
          <div className="fields">
            <label>
              Buyer name
              <input
                value={buyerName}
                onChange={(event) => setBuyerName(event.target.value)}
                autoComplete="name"
                maxLength={200}
              />
            </label>
            <label>
              Buyer email
              <input
                type="email"
                value={buyerEmail}
                onChange={(event) => setBuyerEmail(event.target.value)}
                autoComplete="email"
              />
            </label>
          </div>

          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Product</th>
                  <th>Price</th>
                  <th>In stock</th>
                  <th>Quantity</th>
                </tr>
              </thead>
              <tbody>
                {products.data.map((product) => (
                  <tr key={product.id}>
                    <td>{product.name}</td>
                    <td>{formatMoney(product.priceMinor, product.currency)}</td>
                    <td>{product.stock}</td>
                    <td>
                      <input
                        className="qty"
                        inputMode="numeric"
                        aria-label={`Quantity for ${product.name}`}
                        value={quantities[product.id] ?? ""}
                        onChange={(event) =>
                          setQuantities((current) => ({
                            ...current,
                            [product.id]: event.target.value,
                          }))
                        }
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {selection.lines.length > 0 && !selection.error && (
            <p className="total">
              Request total {formatMoney(selection.totalMinor, selection.currency)}
            </p>
          )}

          {(formError || selection.error || submit.isError) && (
            <p className="banner error" role="alert">
              {formError || selection.error || submit.error?.message}
            </p>
          )}

          <button className="primary" type="submit" disabled={submit.isPending}>
            {submit.isPending ? "Sending…" : "Submit request"}
          </button>
        </form>
      )}
    </section>
  );
}

function LineTable({ lines }: { lines: OrderRequest["lines"] }) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Product</th>
            <th>Qty</th>
            <th>Price</th>
            <th>Line</th>
          </tr>
        </thead>
        <tbody>
          {lines.map((line) => (
            <tr key={line.productId}>
              <td>{line.productName}</td>
              <td>{line.quantity}</td>
              <td>{formatMoney(line.unitPriceMinor, line.currency)}</td>
              <td>{formatMoney(line.lineTotalMinor, line.currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
