import { useNavigate } from "react-router-dom";
import { Login } from "../components/Login";

export function LoginPage() {
  const navigate = useNavigate();
  return <Login onLoggedIn={() => navigate("/")} />;
}
