import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { orderStatusValues } from "@/lib/enums.gen";
import {
  Badge, Banner, Button, Chips, ConfirmDialog, DataTable, Field, Input, Price, QtyStepper, Sheet, Switch,
  TabPanel, Tabs, ToastProvider, statusTone, useToast,
} from ".";

describe("Button", () => {
  it("is a plain button that does not submit forms by accident", () => {
    render(<Button>Pay</Button>);
    expect(screen.getByRole("button", { name: "Pay" })).toHaveAttribute("type", "button");
  });

  it("cannot be clicked while loading", async () => {
    const onClick = vi.fn();
    render(<Button loading onClick={onClick}>Pay</Button>);
    const button = screen.getByRole("button", { name: /Pay/ });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
    await userEvent.click(button);
    expect(onClick).not.toHaveBeenCalled();
  });
});

describe("Field", () => {
  it("ties label, hint and error to the input", () => {
    render(
      <Field label="Phone" hint="Starts with 07" error="Too short">
        {(p) => <Input {...p} />}
      </Field>,
    );
    const input = screen.getByLabelText("Phone");
    expect(input).toHaveAttribute("aria-invalid", "true");
    const described = (input.getAttribute("aria-describedby") ?? "").split(" ");
    expect(described).toHaveLength(2);
    const texts = described.map((id) => document.getElementById(id)?.textContent);
    expect(texts).toEqual(["Too short", "Starts with 07"]);
  });

  it("is not marked invalid without an error", () => {
    render(<Field label="Name">{(p) => <Input {...p} />}</Field>);
    expect(screen.getByLabelText("Name")).not.toHaveAttribute("aria-invalid");
  });
});

describe("QtyStepper", () => {
  it("stops at the limit and at the minimum", async () => {
    const onChange = vi.fn();
    const { rerender } = render(<QtyStepper value={2} max={2} onChange={onChange} />);
    expect(screen.getByRole("button", { name: "Add one" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Remove one" }));
    expect(onChange).toHaveBeenCalledWith(1);
    rerender(<QtyStepper value={0} max={2} onChange={onChange} />);
    expect(screen.getByRole("button", { name: "Remove one" })).toBeDisabled();
  });
});

describe("Price", () => {
  it("shows the MRP struck through and what you save", () => {
    render(<Price cents={143500} mrpCents={150000} showSave />);
    expect(screen.getByText("Rs 1,435.00")).toBeInTheDocument();
    expect(screen.getByText("Rs 1,500.00").tagName).toBe("S");
    expect(screen.getByText("Save Rs 65")).toBeInTheDocument();
  });

  it("shows no saving when the MRP is not higher", () => {
    render(<Price cents={1000} mrpCents={1000} showSave />);
    expect(screen.queryByText(/Save/)).toBeNull();
  });
});

describe("Switch", () => {
  it("reports the new value", async () => {
    const onChange = vi.fn();
    render(<Switch label="Accepting orders" checked={false} onChange={onChange} />);
    await userEvent.click(screen.getByRole("switch", { name: "Accepting orders" }));
    expect(onChange).toHaveBeenCalledWith(true);
  });
});

describe("dialogs", () => {
  it("renders nothing when closed and a labelled dialog when open", () => {
    const { rerender } = render(<Sheet open={false} onClose={() => {}} title="Cart">body</Sheet>);
    expect(screen.queryByRole("dialog", { hidden: true })).toBeNull();
    rerender(<Sheet open onClose={() => {}} title="Cart">body</Sheet>);
    expect(screen.getByRole("dialog", { name: "Cart" })).toBeInTheDocument();
  });

  it("asks the parent to close on Esc and on the close button", async () => {
    const onClose = vi.fn();
    render(<Sheet open onClose={onClose} title="Cart">body</Sheet>);
    const dialog = screen.getByRole("dialog", { name: "Cart" });
    dialog.dispatchEvent(new Event("cancel", { cancelable: true }));
    expect(onClose).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("ConfirmDialog confirms or cancels", async () => {
    const onConfirm = vi.fn();
    const onClose = vi.fn();
    render(<ConfirmDialog open title="Clear cart" message="Sure?" confirmLabel="Clear" danger onConfirm={onConfirm} onClose={onClose} />);
    await userEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("gives the page its scroll back when it closes", () => {
    const { rerender } = render(<Sheet open onClose={() => {}} title="Cart">x</Sheet>);
    expect(document.documentElement.style.overflow).toBe("hidden");
    rerender(<Sheet open={false} onClose={() => {}} title="Cart">x</Sheet>);
    expect(document.documentElement.style.overflow).toBe("");
  });
});

describe("Tabs", () => {
  const tabs = [{ id: "a", label: "New" }, { id: "b", label: "Done" }] as const;

  it("moves with the arrow keys and links to its panel", async () => {
    const onChange = vi.fn();
    render(
      <>
        <Tabs tabs={tabs} value="a" onChange={onChange} label="Orders" idPrefix="o" />
        <TabPanel idPrefix="o" id="a" active>first</TabPanel>
      </>,
    );
    const tab = screen.getByRole("tab", { name: "New" });
    expect(tab).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveAttribute("aria-labelledby", tab.id);
    tab.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenCalledWith("b");
  });
});

describe("Chips", () => {
  it("toggles off when the chosen chip is tapped again", async () => {
    const onChange = vi.fn();
    render(<Chips label="Categories" items={[{ id: "rice", label: "Rice" }]} value="rice" onChange={onChange} />);
    const chip = screen.getByRole("button", { name: "Rice" });
    expect(chip).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(chip);
    expect(onChange).toHaveBeenCalledWith(null);
  });
});

describe("DataTable", () => {
  type Row = { id: string; name: string };
  const columns = [{ key: "name", header: "Name", cell: (r: Row) => r.name }];

  it("shows rows, and opens one with Enter", async () => {
    const onRowClick = vi.fn();
    render(<DataTable caption="Orders" columns={columns} rows={[{ id: "1", name: "Rice" }]} rowKey={(r) => r.id} onRowClick={onRowClick} />);
    const row = screen.getByRole("row", { name: /Rice/ });
    row.focus();
    await userEvent.keyboard("{Enter}");
    expect(onRowClick).toHaveBeenCalledWith({ id: "1", name: "Rice" });
  });

  it("shows an empty state and a loading state", () => {
    const { rerender } = render(<DataTable caption="Orders" columns={columns} rows={[]} rowKey={(r: Row) => r.id} emptyTitle="No orders" />);
    expect(screen.getByText("No orders")).toBeInTheDocument();
    rerender(<DataTable caption="Orders" columns={columns} rows={[]} rowKey={(r: Row) => r.id} loading />);
    expect(screen.queryByText("No orders")).toBeNull();
    expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(6); // header + 5 skeleton rows
  });
});

describe("Banner and Badge", () => {
  it("interrupts a screen reader only for errors", () => {
    const { rerender } = render(<Banner tone="danger">Payment failed</Banner>);
    expect(screen.getByRole("alert")).toHaveTextContent("Payment failed");
    rerender(<Banner>Saved</Banner>);
    expect(screen.getByRole("status")).toHaveTextContent("Saved");
  });

  it("renders a badge", () => {
    render(<Badge tone="success">Paid</Badge>);
    expect(screen.getByText("Paid")).toBeInTheDocument();
  });
});

describe("statusTone", () => {
  it("has a tone for every order status in the contract", () => {
    for (const status of orderStatusValues) expect(statusTone(status)).toBeTruthy();
  });
});

describe("Toast", () => {
  function Trigger() {
    const toast = useToast();
    return <button onClick={() => toast.show("Cart updated")}>go</button>;
  }

  it("announces in a polite live region and goes away by itself", () => {
    vi.useFakeTimers();
    render(<ToastProvider><Trigger /></ToastProvider>);
    act(() => screen.getByRole("button", { name: "go" }).click());
    const region = screen.getByText("Cart updated").parentElement;
    expect(region).toHaveAttribute("aria-live", "polite");
    act(() => vi.advanceTimersByTime(4100));
    expect(screen.queryByText("Cart updated")).toBeNull();
    vi.useRealTimers();
  });

  it("needs its provider", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => render(<Trigger />)).toThrow(/ToastProvider/);
    spy.mockRestore();
  });
});
