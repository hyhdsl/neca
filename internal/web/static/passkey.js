// 本文件处理 necap Web 登录的 Passkey 浏览器断言。
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
    return btoa(s).replace(/+/g, "-").replace(///g, "_").replace(/=+$/, "");
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
  var button = document.getElementById("passkey-login");
  if (!button) { return; }
  if (!window.PublicKeyCredential || !navigator.credentials) {
    button.disabled = true;
    button.textContent = "Passkey unsupported";
    return;
  }
  var error = document.getElementById("passkey-error");
  function showError(message) {
    if (error) { error.hidden = false; error.textContent = message; }
  }
  button.addEventListener("click", function () {
    button.disabled = true;
    if (error) { error.hidden = true; }
    post(button.dataset.begin, {}, button.dataset.csrf).then(function (begin) {
      var options = begin.options.publicKey;
      options.challenge = decode(options.challenge);
      if (options.allowCredentials) {
        options.allowCredentials = options.allowCredentials.map(function (item) {
          return {type: item.type, id: decode(item.id), transports: item.transports};
        });
      }
      return navigator.credentials.get({publicKey: options}).then(function (credential) {
        var response = credential.response;
        return post(button.dataset.finish, {
          challengeId: begin.challengeId,
          response: {
            id: credential.id,
            rawId: encode(credential.rawId),
            type: credential.type,
            response: {
              clientDataJSON: encode(response.clientDataJSON),
              authenticatorData: encode(response.authenticatorData),
              signature: encode(response.signature),
              userHandle: response.userHandle ? encode(response.userHandle) : null
            }
          }
        }, button.dataset.csrf);
      });
    }).then(function (result) {
      window.location.assign(result.redirect || "/");
    }).catch(function () {
      showError("Passkey 验证失败或已取消");
      button.disabled = false;
    });
  });
})();