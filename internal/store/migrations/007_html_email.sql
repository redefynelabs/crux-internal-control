-- Designed (HTML) emails. A template is plain text or HTML; an HTML template holds the message content only (headings,
-- paragraphs, lists, links and the [[button: Label | URL]], [[steps: a | b]] and [[details: Label = value | ...]]
-- shortcodes) and is wrapped in the branded layout when it is sent, together with an automatic plain-text copy.
ALTER TABLE email_templates
    ADD COLUMN format TEXT NOT NULL DEFAULT 'text',
    ADD CONSTRAINT email_templates_format_check CHECK (format IN ('text', 'html'));

-- The HTML part of a queued message; empty for plain text. body stays the plain text shown in the lead's history.
ALTER TABLE emails ADD COLUMN body_html TEXT NOT NULL DEFAULT '';

-- The five starter templates become designed emails, but only where nobody has edited them (they still start as the
-- original plain text).
UPDATE email_templates SET format = 'html', updated_at = now(), body = $b$<h1>Thanks, {{first_name}}. We have your message.</h1>
<p>Someone from the team will come back to you within one working day.</p>
<p>While you wait, the quickest way to see if Crux fits is to try it. You get a private copy with a sample company already loaded: no card, no setup, ready in about ten seconds.</p>
[[button: Start your free demo | {{site_url}}/demo]]
<p>{{signature}}</p>$b$
WHERE key = 'contact_received' AND format = 'text' AND body LIKE 'Hi {{first_name}},%';

UPDATE email_templates SET format = 'html', updated_at = now(), body = $b$<h1>How was the demo, {{first_name}}?</h1>
<p>I hope you had a chance to look around and ask Mei a question or two. The most useful next step is usually a short walkthrough built around how you work.</p>
[[button: Book a 20-minute walkthrough | mailto:{{sender_email}}?subject=Crux%20walkthrough]]
<p>Prefer to start with numbers? Here are the <a href="{{site_url}}/pricing">plans and pricing</a>. Or just reply with what you would like to see for {{company}}; I read every reply.</p>
<p>{{signature}}</p>$b$
WHERE key = 'demo_followup' AND format = 'text' AND body LIKE 'Hi {{first_name}},%';

UPDATE email_templates SET format = 'html', updated_at = now(), body = $b$<h1>Crux on our cloud plan</h1>
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
<p>{{signature}}</p>$b$
WHERE key = 'cloud_plan' AND format = 'text' AND body LIKE 'Hi {{first_name}},%';

UPDATE email_templates SET format = 'html', updated_at = now(), body = $b$<h1>Your proposal for {{company}}</h1>
<p>Hi {{first_name}}, as discussed, here is the proposal. Have a look and tell me what you would change. I am happy to jump on a call to go through it.</p>
[[button: Reply with your questions | mailto:{{sender_email}}?subject=Crux%20proposal]]
<p>{{signature}}</p>$b$
WHERE key = 'proposal_sent' AND format = 'text' AND body LIKE 'Hi {{first_name}},%';

UPDATE email_templates SET format = 'html', updated_at = now(), body = $b$<p>Hi {{first_name}},</p>
<p>I wanted to check in. Is there anything I can answer, or anything blocking you from moving ahead?</p>
<p>If the timing has changed, just tell me and I will follow up later. A one-line reply is plenty.</p>
[[button: Reply to this email | mailto:{{sender_email}}?subject=Re%3A%20Crux]]
<p>{{signature}}</p>$b$
WHERE key = 'check_in' AND format = 'text' AND body LIKE 'Hi {{first_name}},%';

-- Sent by the control plane itself: the login of a demo that was waiting for a room, and the apology when it could not start.
INSERT INTO email_templates (key, name, subject, format, body) VALUES
('demo_ready', 'Demo ready (after waiting)', 'Your Crux demo is ready, {{first_name}}', 'html', $b$<h1>Your demo is ready</h1>
<p>Hi {{first_name}}, a room just opened up and your private copy of Crux is live, with a sample company already loaded. Everything works: click through it, change things, break things.</p>
[[details: Email = {{login_email}} | Password = {{login_password}} | Ends at = {{ends_at}}]]
[[button: Open your demo | {{demo_url}}]]
<h2>Try this first</h2>
[[steps: Open the dashboard to see the whole company at a glance | Ask Mei, the AI assistant, a question about the numbers | Add a lead and move it through the pipeline]]
<p class="muted">Your demo runs for {{minutes}} minutes and is deleted afterwards. Please do not enter real customer or employee data.</p>
<p>Want a guided tour instead? Reply to this email and we will set one up.</p>$b$),
('demo_queue_sorry', 'Demo could not start', 'We could not start your demo', 'html', $b$<h1>Sorry, we could not start your demo</h1>
<p>Hi {{first_name}}, we are sorry: all our demo rooms stayed busy, or something went wrong on our side, so your demo did not start this time.</p>
<p>You can ask for a new demo any time. It starts straight away when a room is free.</p>
[[button: Request a new demo | {{site_url}}/demo]]
<p>Would you rather talk to a person? Reply to this email and we will show you around ourselves.</p>$b$)
ON CONFLICT (key) DO NOTHING;
