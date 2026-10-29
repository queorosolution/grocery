import { ConsoleOrderScreen } from "@/features/console-shop/ConsoleOrderScreen";

export function generateStaticParams() {
  return [{ shopId: "_", orderId: "_" }];
}

export default function Page() {
  return <ConsoleOrderScreen />;
}
