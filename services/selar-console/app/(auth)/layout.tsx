// layout.tsx — Auth layout for login and register pages.
// Centered, clean layout without the Topbar for unauthenticated users.

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="selar-app" style={{ justifyContent: "center", alignItems: "center", background: "var(--bg)" }}>
      {children}
    </div>
  );
}
