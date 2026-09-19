export function FormField({ label, type = 'text', value, onChange, placeholder, required, autoFocus }) {
  return (
    <div className="flex flex-col gap-1">
      <label className="font-body text-[11px] tracking-[0.44px] uppercase text-ink-soft">{label}</label>
      <input
        type={type}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
        required={required}
        autoFocus={autoFocus}
        className="w-full rounded-md bg-surface px-4 py-2.5 font-body text-sm text-ink shadow-[inset_0px_1px_2px_0px_rgba(0,0,0,0.05)] outline-none transition-shadow focus:ring-2 focus:ring-brand/40"
      />
    </div>
  );
}
