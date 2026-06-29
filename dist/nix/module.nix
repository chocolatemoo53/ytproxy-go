self:
{
  config,
  pkgs,
  lib,
  ...
}:
let
  cfg = config.services.ytproxy-go;
  # Import the package from the flake
  settingsFormat = pkgs.formats.json { };
  settingsFile = settingsFormat.generate "ytproxy-go-settings.yaml" cfg.settings;

  ytproxy-go = self.packages.${pkgs.system}.default;
in
{
  options.services.ytproxy-go = {
    enable = lib.mkEnableOption "Enable ytproxy-go service.";

    package = lib.mkOption {
      type = lib.types.package;
      default = ytproxy-go;
      description = "The ytproxy-go package to use.";
    };

    user = lib.mkOption {
      type = lib.types.str;
      default = "ytproxy-go";
      description = "User account under which ytproxy-go runs.";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "ytproxy-go";
      description = "Group under which ytproxy-go runs.";
    };

    settings = lib.mkOption {
      type = settingsFormat.type;
      default = { };
      description = ''
        The settings ytproxy-go should use.

        See [config.example.yaml](https://git.nadeko.net/Fijxu/ytproxy-go/src/branch/master/config/config.example.yaml) for a list of all possible options.
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
  #             description = "Enables the http server of ytproxy-go";
  #           };
  #           unixSocket = lib.mkOption {
  #             type = lib.types.bool;
  #             default = true;
  #             description = "Unix sockets server";
  #           };
  #           unixSocketPath = lib.mkOption {
  #             type = lib.types.string;
  #             default = "/tmp/ytproxy-go.sock";
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
  #     description = "The ytproxy-go config";
  #   };
  # };

  config = lib.mkIf cfg.enable {
    users.users = lib.mkIf (cfg.user == "ytproxy-go") {
      ytproxy-go = {
        description = "ytproxy-go user";
        isSystemUser = true;
        group = cfg.group;
      };
    };

    users.groups = lib.mkIf (cfg.group == "ytproxy-go") {
      ytproxy-go = { };
    };

    systemd.services.ytproxy-go = {
      description = "Youtube /videoplayback proxy";
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];
      wantedBy = [ "multi-user.target" ];

      environment = {
        YTPROXY_CONFIG_PATH = settingsFile;
      };

      serviceConfig = {
        ExecStart = "${cfg.package}/bin/ytproxy-go";
        DynamicUser = true;
        User = cfg.user;
        Group = cfg.group;
        Restart = "on-failure";
      };
    };
  };
}
