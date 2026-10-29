import { AdminGate } from "@/features/auth/guards";

// Super admins past the authenticator step. Admin screens arrive in P33.
export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return <AdminGate>{children}</AdminGate>;
}
