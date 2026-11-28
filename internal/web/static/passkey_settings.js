// 本文件处理 necap Web 设置中的 Passkey 注册。
(function () {
  "use strict";
  function decode(value) {
    var s = value.replace(/-/g, "+").replace(/_/g, "/");
    while (s.length % 4) { s += "="; }
    var raw = atob(s);
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) { out[i] = raw.charCodeAt(i); }
    return out;
  }
  function encode(buffer) {
    var bytes = new Uint8Array(buffer);
    var s = "";
    for (var i = 0; i < bytes.length; i++) { s += String.fromCharCode(bytes[i]); }
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }
  function post(url, body, csrf) {
    return fetch(url, {
      method: "POST",
      credentials: "same-origin",
      headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf},
      body: JSON.stringify(body || {})
    }).then(function (response) {
      if (!response.ok) { throw new Error("request failed"); }
      return response.json();
    });
  }
  var button = document.getElementById("passkey-register");
  if (!button) { return; }
  if (!window.PublicKeyCredential || !navigator.credentials) {
    button.disabled = true;
    return;
  }
  var error = document.getElementById("passkey-settings-error");
  var recovery = document.getElementById("passkey-recovery");
  var recoveryCodes = document.getElementById("passkey-recovery-codes");
  function showError(message) {
    if (error) { error.hidden = false; error.textContent = message; }
  }
  button.addEventListener("click", function () {
    button.disabled = true;
    if (error) { error.hidden = true; }
    post(button.dataset.begin, {}, button.dataset.csrf).then(function (begin) {
      var options = begin.options.publicKey;
      options.challenge = decode(options.challenge);
      options.user.id = decode(options.user.id);
      if (options.excludeCredentials) {
        options.excludeCredentials = options.excludeCredentials.map(function (item) {
          return {type: item.type, id: decode(item.id), transports: item.transports};
        });
      }
      return navigator.credentials.create({publicKey: options}).then(function (credential) {
        var response = credential.response;
        return post(button.dataset.finish, {
          challengeId: begin.challengeId,
          label: "Web Passkey",
          response: {
            id: credential.id,
            rawId: encode(credential.rawId),
            type: credential.type,
            response: {
              clientDataJSON: encode(response.clientDataJSON),
              attestationObject: encode(response.attestationObject),
              transports: response.getTransports ? response.getTransports() : []
            }
          }
        }, button.dataset.csrf);
      });
    }).then(function (result) {
      button.disabled = false;
      if (result.recoveryCodes && result.recoveryCodes.length) {
        if (recovery) { recovery.hidden = false; }
        if (recoveryCodes) { recoveryCodes.textContent = result.recoveryCodes.join("\n"); }
      } else {
        window.location.reload();
      }
    }).catch(function () {
      showError("Passkey 注册失败或已取消");
      button.disabled = false;
    });
  });
})();