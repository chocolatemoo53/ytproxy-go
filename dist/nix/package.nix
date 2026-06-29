{ buildGoModule }:

buildGoModule rec {
  pname = "ytproxy-go";
  version = "0.1.0";
  vendorHash = "sha256-JMHXL+bLilc0y9ZJZ7Mbd7HpqrQMCJLfw6Ob5/g4fMk=";
  src = ./../../.;
}
