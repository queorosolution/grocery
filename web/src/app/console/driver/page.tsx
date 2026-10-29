import { RequireRole } from "@/features/auth/guards";
import { DriverScreen } from "@/features/driver/DriverScreen";

export default function Page() {
  return (
    <RequireRole roles={["SHOP_DRIVER", "PLATFORM_DRIVER"]}>
      <DriverScreen />
    </RequireRole>
  );
}
