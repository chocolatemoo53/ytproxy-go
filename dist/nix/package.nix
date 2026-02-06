{ buildGoModule }:

buildGoModule rec {
  pname = "http3-ytproxy";
  version = "0.1.0";
  vendorHash = "sha256-JMHXL+bLilc0y9ZJZ7Mbd7HpqrQMCJLfw6Ob5/g4fMk=";
  src = ./../../.;
}
