-- Designed emails take the website's look: a small uppercase label above the heading, like the site's "FREE DEMO". Only
-- templates that still have the text the previous release gave them are changed (matched by hash), so edits are never lost.

UPDATE email_templates SET body = $b$[[eyebrow: Message received]]
<h1>Thanks, {{first_name}}. We have your message.</h1>
<p>Someone from the team will come back to you within one working day.</p>
<p>While you wait, the quickest way to see if Crux fits is to try it. You get a private copy with a sample company already loaded: no card, no setup, ready in about ten seconds.</p>
[[button: Start your free demo | {{site_url}}/demo]]
<p>{{signature}}</p>$b$, updated_at = now()
WHERE key = 'contact_received' AND format = 'html' AND md5(body) = '87c7344fcd5b1e9626e84011f162e0cd';

UPDATE email_templates SET body = $b$[[eyebrow: Your demo]]
<h1>How was the demo, {{first_name}}?</h1>
<p>I hope you had a chance to look around and ask Mei a question or two. The most useful next step is usually a short walkthrough built around how you work.</p>
[[button: Book a 20-minute walkthrough | mailto:{{sender_email}}?subject=Crux%20walkthrough]]
<p>Prefer to start with numbers? Here are the <a href="{{site_url}}/pricing">plans and pricing</a>. Or just reply with what you would like to see for {{company}}; I read every reply.</p>
<p>{{signature}}</p>$b$, updated_at = now()
WHERE key = 'demo_followup' AND format = 'html' AND md5(body) = '43a3cbfb8b1d25993642476a4b1c9828';

UPDATE email_templates SET body = $b$[[eyebrow: Cloud plan]]
<h1>Crux on our cloud plan</h1>
<p>Hi {{first_name}}, thanks for your interest. We host it and look after it for you, so there is nothing to install or maintain.</p>
<ul>
<li><strong>One flat price</strong> by company size</li>
<li><strong>Every feature</strong> and unlimited users</li>
<li><strong>Daily backups</strong>, so your data is safe</li>
<li><strong>Your own separate instance</strong>, shared with nobody</li>
</ul>
<p>Tell me roughly how many people you have and I will suggest the right size and send a quote.</p>
[[button: Get my quote | mailto:{{sender_email}}?subject=Crux%20cloud%20plan%20quote]]
<p class="muted">Want to compare first? See the <a href="{{site_url}}/pricing">pricing page</a>.</p>
<p>{{signature}}</p>$b$, updated_at = now()
WHERE key = 'cloud_plan' AND format = 'html' AND md5(body) = '71c1f2e040130129415832c3b42f7a90';

UPDATE email_templates SET body = $b$[[eyebrow: Proposal]]
<h1>Your proposal for {{company}}</h1>
<p>Hi {{first_name}}, as discussed, here is the proposal. Have a look and tell me what you would change. I am happy to jump on a call to go through it.</p>
[[button: Reply with your questions | mailto:{{sender_email}}?subject=Crux%20proposal]]
<p>{{signature}}</p>$b$, updated_at = now()
WHERE key = 'proposal_sent' AND format = 'html' AND md5(body) = 'd50ef7799cd40825f8952dcd9b3b9620';

UPDATE email_templates SET body = $b$[[eyebrow: Demo ready]]
<h1>Your own copy of Crux is live</h1>
<p>Hi {{first_name}}, a room just opened up and your private copy of Crux is live, with a sample company already loaded. Everything works: click through it, change things, break things.</p>
[[details: Email = {{login_email}} | Password = {{login_password}} | Ends at = {{ends_at}}]]
[[button: Open your demo | {{demo_url}}]]
<h2>Try this first</h2>
[[steps: Open the dashboard to see the whole company at a glance | Ask Mei, the AI assistant, a question about the numbers | Add a lead and move it through the pipeline]]
<p class="muted">Your demo runs for {{minutes}} minutes and is deleted afterwards. Please do not enter real customer or employee data.</p>
<p>Want a guided tour instead? Reply to this email and we will set one up.</p>$b$, updated_at = now()
WHERE key = 'demo_ready' AND format = 'html' AND md5(body) = 'e0a9d3de5a330b7e01eb3f826ed4ac23';

UPDATE email_templates SET body = $b$[[eyebrow: Demo]]
<h1>Sorry, we could not start your demo</h1>
<p>Hi {{first_name}}, we are sorry: all our demo rooms stayed busy, or something went wrong on our side, so your demo did not start this time.</p>
<p>You can ask for a new demo any time. It starts straight away when a room is free.</p>
[[button: Request a new demo | {{site_url}}/demo]]
<p>Would you rather talk to a person? Reply to this email and we will show you around ourselves.</p>$b$, updated_at = now()
WHERE key = 'demo_queue_sorry' AND format = 'html' AND md5(body) = '22e62b108c7e1601ff6e379f2412e949';
