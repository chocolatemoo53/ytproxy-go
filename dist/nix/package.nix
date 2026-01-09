{ buildGoModule }:

buildGoModule rec {
  pname = "http3-ytproxy";
  version = "0.1.0";
  vendorHash = "sha256-wXbDKE+MlsFa5gDOBSLUodzFP6k2rDK2YpR0ggCGv4E=";
  src = ./.;
}
