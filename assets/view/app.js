(function() {
  'use strict';

  // The viewer page is server-rendered with strict escaping, and the server's
  // CSP is script-src 'none', so this script never executes when served by
  // `zbrain view`. It documents the interactive fallback for the /api JSON
  // endpoints: every user-controlled value is written with textContent
  // (never innerHTML) so untrusted claim bodies and evidence bytes stay
  // escaped even if the CSP is ever relaxed.

  var app = document.getElementById('app');
  if (!app) return;

  function renderWorkspace(workspace) {
    var status = document.querySelector('.status');
    if (status) status.textContent = 'trusted memory viewer · workspace: ' + workspace;
  }

  function renderClaims(claims) {
    var section = document.getElementById('claims');
    if (!section) return;
    var list = document.createElement('ul');
    list.className = 'claims';

    claims.forEach(function(claim) {
      var item = document.createElement('li');
      item.className = 'claim';
      item.id = claim.ID;

      var title = document.createElement('h3');
      title.className = 'claim-title';
      title.textContent = claim.Title;
      item.appendChild(title);

      var meta = document.createElement('p');
      meta.className = 'claim-meta';
      meta.textContent = claim.ID + ' · ' + claim.Tier + ' · ' + claim.Status;
      item.appendChild(meta);

      var body = document.createElement('div');
      body.className = 'claim-body';
      body.textContent = claim.Body;
      item.appendChild(body);

      list.appendChild(item);
    });

    section.appendChild(list);
  }

  function getJSON(url) {
    return fetch(url).then(function(response) {
      if (!response.ok) throw new Error(url + ' failed: ' + response.status);
      return response.json();
    });
  }

  getJSON('/api/workspace').then(function(data) {
    renderWorkspace(data.workspace);
  }).catch(function() {});

  getJSON('/api/claims').then(renderClaims).catch(function() {});
})();
