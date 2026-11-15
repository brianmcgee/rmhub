# Starts the NixOS module with examples/nixos.nix in a VM. rmapi pairs
# through TLS with a self-signed CA, uploads a PDF, and lists it again
# after a service restart. The API renders the PDF page under the systemd
# hardening of the module.
{ pkgs, flake, ... }:
let
  # A self-signed CA and a certificate for rmhub.test. The key is in the
  # Nix store, which is acceptable for a test only.
  certs = pkgs.runCommand "rmhub-test-certs" { nativeBuildInputs = [ pkgs.openssl ]; } ''
    mkdir $out
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 3650 \
      -subj /CN=rmhub-test-ca -keyout ca.key -out $out/ca.pem
    openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
      -subj /CN=rmhub.test -keyout $out/key.pem -out server.csr
    printf 'subjectAltName=DNS:rmhub.test\n' > ext
    openssl x509 -req -in server.csr -CA $out/ca.pem -CAkey ca.key -CAcreateserial \
      -days 3650 -extfile ext -out $out/cert.pem
  '';

  # A PDF with one page and a black rectangle. The script writes correct
  # cross-reference offsets.
  pdf =
    pkgs.runCommand "rmhub-test.pdf" { nativeBuildInputs = [ pkgs.python3 ]; } # sh
      ''
        python3 - > $out <<'EOF'
        import sys
        content = b"0 0 0 rg 100 600 200 100 re f"
        objs = [
            b"<< /Type /Catalog /Pages 2 0 R >>",
            b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
            b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
            b"<< /Length %d >>\nstream\n%s\nendstream" % (len(content), content),
        ]
        out = bytearray(b"%PDF-1.4\n")
        offs = []
        for i, o in enumerate(objs):
            offs.append(len(out))
            out += b"%d 0 obj\n%s\nendobj\n" % (i + 1, o)
        xref = len(out)
        out += b"xref\n0 %d\n0000000000 65535 f \n" % (len(objs) + 1)
        for o in offs:
            out += b"%010d 00000 n \n" % o
        out += b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objs) + 1, xref)
        sys.stdout.buffer.write(out)
        EOF
      '';
in
pkgs.testers.runNixOSTest {
  name = "rmhub-nixos-module";

  nodes.machine =
    { lib, ... }:
    {
      imports = [
        flake.nixosModules.rmhub
        ../examples/nixos.nix
      ];

      # The VM has no internet, so it uses the test CA instead of ACME.
      security.acme.certs = lib.mkForce { };
      security.pki.certificateFiles = [ "${certs}/ca.pem" ];
      networking.hosts."127.0.0.1" = [ "rmhub.test" ];

      services.rmhub = {
        credentials = {
          admin-token = lib.mkForce "/etc/rmhub-test/admin-token";
          tls-cert = lib.mkForce "${certs}/cert.pem";
          tls-key = lib.mkForce "${certs}/key.pem";
        };
      };

      # A fixed admin token for the test. A real host gets it from a
      # secret store.
      environment.etc."rmhub-test/admin-token" = {
        text = "rmh_admin_ywgfq67uig2zyv5ya7vvaya2l3mcpcmmwaduixemaoayxspl3fpi\n";
        mode = "0400";
      };

      environment.systemPackages = [
        pkgs.rmapi
        pkgs.jq
        pkgs.curl
        pkgs.file
      ];
    };

  testScript = ''
    machine.wait_for_unit("rmhub.service")
    machine.wait_until_succeeds("curl -sf http://127.0.0.1:8081/healthz")
    machine.wait_until_succeeds("curl -sf https://rmhub.test:8443/healthz")

    print(machine.succeed("systemd-analyze security rmhub.service --no-pager | tail -n 1"))

    api = "RMHUB_URL=http://127.0.0.1:8081"
    admin = f"{api} RMHUB_TOKEN=$(cat /etc/rmhub-test/admin-token)"
    account = machine.succeed(f"{admin} rmhub admin user add --email ada@example.org --json | jq -r .id").strip()
    owner = machine.succeed(f"{admin} rmhub admin token issue {account}").strip()
    user = f"{api} RMHUB_TOKEN={owner}"

    rmapi = "HOME=/root RMAPI_HOST=https://rmhub.test:8443 rmapi"
    code = machine.succeed(f"{user} rmhub pair --json | jq -r .code").strip()
    machine.succeed(f"echo {code} | {rmapi} ls")
    machine.succeed("cp ${pdf} /root/paper.pdf")
    machine.succeed(f"{rmapi} put /root/paper.pdf /")
    assert "paper" in machine.succeed(f"{rmapi} ls /")

    doc = machine.succeed(f"{user} rmhub ls / --json | jq -r '.[] | select(.name == \"paper\") | .id'").strip()
    machine.succeed(f"{user} rmhub render {doc} --page 1 -o /root/page.png")
    assert "PNG image data, 1404 x" in machine.succeed("file /root/page.png")

    machine.systemctl("restart rmhub.service")
    machine.wait_for_unit("rmhub.service")
    machine.wait_until_succeeds("curl -sf https://rmhub.test:8443/healthz")
    assert "paper" in machine.succeed(f"{rmapi} ls /")
    machine.fail("grep -r rmh_admin_ywgfq67 /nix/store/*rmhub.toml")
  '';
}
