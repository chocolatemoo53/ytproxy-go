self:
{
  config,
  pkgs,
  lib,
  ...
}:
let
  cfg = config.services.http3-ytproxy;
  # Import the package from the flake
  settingsFormat = pkgs.formats.json { };
  settingsFile = settingsFormat.generate "http3-ytproxy-settings.yaml" cfg.settings;

  http3-ytproxy = self.packages.${pkgs.system}.default;
in
{
  options.services.http3-ytproxy = {
    enable = lib.mkEnableOption "Enable http3-ytproxy service.";

    package = lib.mkOption {
      type = lib.types.package;
      default = http3-ytproxy;
      description = "The http3-ytproxy package to use.";
    };

    user = lib.mkOption {
      type = lib.types.str;
      default = "http3-ytproxy";
      description = "User account under which http3-ytproxy runs.";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "http3-ytproxy";
      description = "Group under which http3-ytproxy runs.";
    };

    settings = lib.mkOption {
      type = settingsFormat.type;
      default = { };
      description = ''
        The settings http3-ytproxy should use.

        See [config.example.yaml](https://git.nadeko.net/Fijxu/http3-ytproxy/src/branch/master/config/config.example.yaml) for a list of all possible options.
      '';
    };
  };

  #   settings = lib.mkOption {
  #     type = lib.types.submodule {
  #       server = lib.mkOption {
  #         type = lib.types.submodule {
  #           enableHttp = lib.mkOption {
  #             type = lib.types.bool;
  #             default = true;
  #             description = "Enables the http server of http3-ytproxy";
  #           };
  #           unixSocket = lib.mkOption {
  #             type = lib.types.bool;
  #             default = true;
  #             description = "Unix sockets server";
  #           };
  #           unixSocketPath = lib.mkOption {
  #             type = lib.types.string;
  #             default = "/tmp/http-ytproxy.sock";
  #             description = "Path of the unix socket";
  #           };
  #           host = lib.mkOption {
  #             type = lib.types.string;
  #             default = "0.0.0.0";
  #             description = "Listening address for the HTTP server";
  #           };
  #           port = lib.mkOption {
  #             type = lib.types.int;
  #             default = 8080;
  #             description = "Listening port for the HTTP server";
  #           };
  #         };
  #       };
  #       proxy = lib.mkOption {
  #         type = lib.types.string;
  #         default = "";
  #         description = "Proxy to use for requests for Youtube";
  #       };
  #       httpClientVersion = lib.mkOption {
  #         type = lib.types.int;
  #         default = 1;
  #         description = ''
  #           # The HTTP client version to use for requests to Youtube
  #           # 1: HTTP/1.1
  #           # 2: HTTP/2
  #           # 3: HTTP/3
  #         '';
  #       };

  #       ipv6Only = lib.mkOption {
  #         type = lib.types.bool;
  #         default = false;
  #         description = "Only use IPV6 for requests to Youtube";
  #       };
  #       logLevel = lib.mkOption {
  #         type = lib.types.string;
  #         default = "info";
  #         description = "Log level, can be: trace, debug, info, warn, error, fatal, panic";
  #       };
  #       companion = lib.mkOption {
  #         type = lib.types.submodule {
  #           secretKey = {
  #             type = lib.types.string;
  #             default = "";
  #             description = "16 characters secret key from Invidious Companion";
  #           };
  #         };
  #       };
  #     };
  #     default = { };
  #     description = "The http3-ytproxy config";
  #   };
  # };

  config = lib.mkIf cfg.enable {
    users.users = lib.mkIf (cfg.user == "http3-ytproxy") {
      http3-ytproxy = {
        description = "http3-ytproxy user";
        isSystemUser = true;
        group = cfg.group;
      };
    };

    users.groups = lib.mkIf (cfg.group == "http3-ytproxy") {
      http3-ytproxy = { };
    };

    systemd.services.http3-ytproxy = {
      description = "Youtube /videoplayback proxy";
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];
      wantedBy = [ "multi-user.target" ];

      environment = {
        YTPROXY_CONFIG_PATH = settingsFile;
      };

      serviceConfig = {
        ExecStart = "${cfg.package}/bin/http3-ytproxy";
        DynamicUser = true;
        User = cfg.user;
        Group = cfg.group;
        Restart = "on-failure";
      };
    };
  };
}
