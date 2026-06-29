{ buildGoModule }:

buildGoModule rec {
  pname = "ytproxy-go";
  version = "0.1.0";
  vendorHash = "sha256-hHCxder8MekLpKjhWN8WR1Jy5HkexY+aHF9yeFKODtM=";
  src = ./../../.;
}
