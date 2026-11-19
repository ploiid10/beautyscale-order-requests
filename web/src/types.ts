export type Product = {
  id: string;
  name: string;
  priceMinor: number;
  currency: string;
  stock: number;
};

export type Line = {
  productId: string;
  productName: string;
  quantity: number;
  unitPriceMinor: number;
  currency: string;
  lineTotalMinor: number;
};

export type OrderRequest = {
  id: string;
  buyerName: string;
  buyerEmail: string;
  status: "new" | "accepted" | "rejected";
  createdAt: string;
  currency: string;
  totalMinor: number;
  lines: Line[];
};

export type OrderRequestPage = {
  items: OrderRequest[];
  limit: number;
  offset: number;
  total: number;
};

export type DraftOrder = {
  id: string;
  orderRequestId: string;
  status: string;
  createdAt: string;
  currency: string;
  totalMinor: number;
  lines: Line[];
};
