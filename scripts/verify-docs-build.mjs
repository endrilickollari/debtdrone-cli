import { access, readFile, readdir } from 'node:fs/promises';
import path from 'node:path';

const outputDirectory = path.resolve('docs-dist');
const site = (process.env.DOCS_SITE ?? 'https://cli.debtdrone.net').replace(/\/$/, '');
const failures = [];

async function readRequired(relativePath) {
  const filePath = path.join(outputDirectory, relativePath);
  try {
    return await readFile(filePath, 'utf8');
  } catch (error) {
    failures.push(`missing ${relativePath}: ${error.message}`);
    return '';
  }
}

function requireText(content, expected, source) {
  if (!content.includes(expected)) {
    failures.push(`${source} does not contain ${JSON.stringify(expected)}`);
  }
}

const [index, sitemapIndex, sitemap, robots, llms, cname, pagefindEntry] = await Promise.all([
  readRequired('index.html'),
  readRequired('sitemap-index.xml'),
  readRequired('sitemap-0.xml'),
  readRequired('robots.txt'),
  readRequired('llms.txt'),
  readRequired('CNAME'),
  readRequired('pagefind/pagefind-entry.json'),
]);

requireText(index, `<link rel="canonical" href="${site}/"`, 'index.html');
requireText(sitemapIndex, `<loc>${site}/sitemap-0.xml</loc>`, 'sitemap-index.xml');
requireText(robots, `Sitemap: ${site}/sitemap-index.xml`, 'robots.txt');
requireText(llms, `[Documentation overview](${site}/)`, 'llms.txt');

if (cname.trim() !== new URL(site).hostname) {
  failures.push(`CNAME must contain only ${new URL(site).hostname}`);
}

const sitemapLocations = [...sitemap.matchAll(/<loc>([^<]+)<\/loc>/g)].map((match) => match[1]);
if (sitemapLocations.length === 0) {
  failures.push('sitemap-0.xml contains no URLs');
}
for (const location of sitemapLocations) {
  if (!location.startsWith(`${site}/`)) {
    failures.push(`sitemap URL uses the wrong origin: ${location}`);
  }
}

try {
  const pagefind = JSON.parse(pagefindEntry);
  if (!(pagefind.languages?.en?.page_count > 0)) {
    failures.push('Pagefind contains no indexed English pages');
  }
  const fragments = await readdir(path.join(outputDirectory, 'pagefind', 'fragment'));
  if (!fragments.some((file) => file.endsWith('.pf_fragment'))) {
    failures.push('Pagefind contains no search fragments');
  }
  await access(path.join(outputDirectory, 'pagefind', 'pagefind.js'));
} catch (error) {
  failures.push(`invalid Pagefind output: ${error.message}`);
}

if (failures.length > 0) {
  console.error('Documentation verification failed:');
  for (const failure of failures) console.error(`- ${failure}`);
  process.exitCode = 1;
} else {
  console.log(`Documentation verified for ${site} (${sitemapLocations.length} sitemap URLs).`);
}
