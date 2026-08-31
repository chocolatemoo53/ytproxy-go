{ buildGoModule }:

buildGoModule rec {
  pname = "ytproxy-go";
  version = "0.1.0";
  vendorHash = "sha256-1/hYgrCRfiDc08tvfGbmnlUII5iTT2ZAQ51kA+RMLIA=";
  src = ./../../.;
}
