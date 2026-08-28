import Icon from "./icons";
import { useTheme } from "../hooks/useTheme";

type ThemeToggleProps = {
  className?: string;
};

export default function ThemeToggle({ className = "" }: ThemeToggleProps) {
  const { theme, toggle } = useTheme();
  const label = theme === "dark" ? "Switch to light theme" : "Switch to dark theme";
  return (
    <button
      type="button"
      className={["icon-button", "theme-toggle", className].filter(Boolean).join(" ")}
      aria-label={label}
      title={label}
      onClick={toggle}
    >
      <Icon name={theme === "dark" ? "sun" : "moon"} />
    </button>
  );
}
