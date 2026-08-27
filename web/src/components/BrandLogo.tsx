type BrandLogoProps = {
  className?: string;
};

export default function BrandLogo({ className = "" }: BrandLogoProps) {
  return (
    <span className={`brand-logo ${className}`.trim()} aria-hidden="true">
      <img
        className="brand-logo-image brand-logo-light"
        src="/assets/redgres-logo-light.png"
        alt=""
      />
      <img
        className="brand-logo-image brand-logo-dark"
        src="/assets/redgres-logo-dark.png"
        alt=""
      />
    </span>
  );
}
