"use client";

import { createContext, useContext } from "react";
import type { CartState } from "./types";

export const CartContext = createContext<CartState | null>(null);

export function useCart(): CartState {
  const state = useContext(CartContext);
  if (!state) throw new Error("useCart needs <CartProvider>");
  return state;
}
