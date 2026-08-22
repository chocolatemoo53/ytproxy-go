{ buildGoModule }:

buildGoModule rec {
  pname = "ytproxy-go";
  version = "0.1.0";
  vendorHash = "sha256-kO6Z7uSeRD6mm3sitnVr+w0wGzbLliAL4evbbrcYLb0=";
  src = ./../../.;
}
