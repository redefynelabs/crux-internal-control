import type { SVGProps } from "react";

// A handful of 24px outline icons, drawn here so the app has no icon dependency.
function Icon(props: Omit<SVGProps<SVGSVGElement>, "d"> & { d: string | string[] }) {
  const { d, ...rest } = props;
  return (
    <svg viewBox="0 0 24 24" width="1em" height="1em" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...rest}>
      {(Array.isArray(d) ? d : [d]).map((p) => (
        <path key={p} d={p} />
      ))}
    </svg>
  );
}

export const DashboardIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M4 4h7v9H4z", "M13 4h7v5h-7z", "M13 11h7v9h-7z", "M4 15h7v5H4z"]} />;
export const UsersIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M16 20v-1a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v1", "M9.5 11a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7z", "M21 20v-1a4 4 0 0 0-3-3.9", "M15.5 4.2a3.5 3.5 0 0 1 0 6.6"]} />;
export const ServerIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M4 4h16v6H4z", "M4 14h16v6H4z", "M8 7h.01", "M8 17h.01"]} />;
export const LogoutIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4", "M16 17l5-5-5-5", "M21 12H9"]} />;
export const ExternalIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M15 3h6v6", "M10 14L21 3", "M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"]} />;
export const ArrowLeftIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M19 12H5", "M12 19l-7-7 7-7"]} />;
export const SearchIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z", "M21 21l-4.3-4.3"]} />;
export const TasksIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M9 11l3 3L22 4", "M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"]} />;
export const MailIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z", "M22 7l-10 6L2 7"]} />;
export const PhoneIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M22 16.9v3a2 2 0 0 1-2.2 2 19.8 19.8 0 0 1-8.6-3.1 19.5 19.5 0 0 1-6-6A19.8 19.8 0 0 1 2.1 4.2 2 2 0 0 1 4.1 2h3a2 2 0 0 1 2 1.7c.1 1 .4 1.9.7 2.8a2 2 0 0 1-.5 2.1L8 9.9a16 16 0 0 0 6 6l1.3-1.3a2 2 0 0 1 2.1-.4c.9.3 1.8.6 2.8.7a2 2 0 0 1 1.7 2z"]} />;
export const NoteIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z", "M14 3v6h6", "M8 13h8", "M8 17h5"]} />;
export const SettingsIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z", "M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"]} />;
export const TemplateIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M4 4h16v5H4z", "M4 13h7v7H4z", "M15 13h5", "M15 17h5"]} />;
export const PlusIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M12 5v14", "M5 12h14"]} />;
export const CheckIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M20 6L9 17l-5-5"]} />;
export const CopyIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M9 9h11v11H9z", "M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1"]} />;
export const AlertIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M12 9v4", "M12 17h.01", "M10.3 3.9L1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"]} />;
export const ListIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M8 6h13", "M8 12h13", "M8 18h13", "M3 6h.01", "M3 12h.01", "M3 18h.01"]} />;
export const BoardIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M4 4h5v16H4z", "M10.5 4h5v10h-5z", "M17 4h3v6h-3z"]} />;
export const ClockIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z", "M12 7v5l3 2"]} />;
export const LockIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M5 11h14v10H5z", "M8 11V7a4 4 0 0 1 8 0v4"]} />;
export const ChartIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M3 3v18h18", "M7 15l4-5 3 3 5-7"]} />;
export const WalletIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M3 7a2 2 0 0 1 2-2h13v4", "M3 7v11a2 2 0 0 0 2 2h14a1 1 0 0 0 1-1v-9a1 1 0 0 0-1-1H5a2 2 0 0 1-2-2z", "M16 14h.01"]} />;
export const MoveIcon = (p: SVGProps<SVGSVGElement>) => <Icon {...p} d={["M5 9l-3 3 3 3", "M9 5l3-3 3 3", "M15 19l-3 3-3-3", "M19 9l3 3-3 3", "M2 12h20", "M12 2v20"]} />;
