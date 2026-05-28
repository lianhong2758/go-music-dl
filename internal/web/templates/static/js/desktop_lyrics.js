(function () {
    const API_ROOT = window.API_ROOT || '/music';
    const timeRe = /\[(\d+):(\d+)\.(\d{1,3})\]/g;
    const fallbackLineDuration = 1200;
    const statePollMS = 180;
    const colorStorageKey = 'musicDlDesktopLyricsColor';

    const body = document.body;
    const lyricsBody = document.getElementById('desktopLyricsBody');
    const colorPanel = document.getElementById('desktopLyricsColorPanel');
    const colorInput = document.getElementById('desktopLyricsCustomColor');
    let currentLyricURL = '';
    let groups = [];
    let activeIndex = -1;
    let latestState = null;
    let animationFrame = 0;

    function escapeHTML(value) {
        return String(value ?? '').replace(/[&<>"']/g, (char) => {
            switch (char) {
            case '&': return '&amp;';
            case '<': return '&lt;';
            case '>': return '&gt;';
            case '"': return '&quot;';
            case '\'': return '&#39;';
            default: return char;
            }
        });
    }

    function setIdle(idle) {
        body.classList.toggle('desktop-lyrics-idle', idle);
        if (idle) {
            groups = [];
            activeIndex = -1;
            lyricsBody.innerHTML = '';
        }
    }

    function timeToMs(parts) {
        const minute = Number(parts[1]) || 0;
        const second = Number(parts[2]) || 0;
        let ms = String(parts[3] || '0');
        if (ms.length === 1) ms += '00';
        if (ms.length === 2) ms += '0';
        return minute * 60000 + second * 1000 + Number(ms.slice(0, 3));
    }

    function parseLine(line) {
        timeRe.lastIndex = 0;
        const matches = Array.from(line.matchAll(timeRe));
        if (matches.length === 0) return null;
        const start = timeToMs(matches[0]);
        const words = [];
        for (let i = 0; i < matches.length; i++) {
            const textStart = matches[i].index + matches[i][0].length;
            const textEnd = i + 1 < matches.length ? matches[i + 1].index : line.length;
            const text = line.slice(textStart, textEnd);
            if (!text) continue;
            words.push({
                start: timeToMs(matches[i]),
                end: i + 1 < matches.length ? timeToMs(matches[i + 1]) : null,
                text
            });
        }
        const text = line.replace(timeRe, '').trim();
        return { start, words, text, verbatim: matches.length > 1 };
    }

    function normalizeGroupWords(sourceWords, groupStart, groupEnd, fallbackText) {
        const words = Array.isArray(sourceWords) && sourceWords.length > 0
            ? sourceWords
            : [{ text: fallbackText || '', start: groupStart, end: groupEnd }];
        return words
            .map((word, index) => {
                const start = Number(word?.start);
                const nextStart = index + 1 < words.length ? Number(words[index + 1]?.start) : NaN;
                let end = Number(word?.end);
                const safeStart = Number.isFinite(start) ? start : groupStart;
                if (!Number.isFinite(end) || end <= safeStart) {
                    end = Number.isFinite(nextStart) && nextStart > safeStart ? nextStart : groupEnd;
                }
                return {
                    text: String(word?.text || ''),
                    start: safeStart,
                    end
                };
            })
            .filter(word => word.text !== '');
    }

    function normalizeGroups(rawGroups) {
        return (rawGroups || []).map((group, index, list) => {
            const start = Number(group?.start || 0);
            const nextStart = index + 1 < list.length ? Number(list[index + 1]?.start || 0) : 0;
            const end = nextStart > start ? nextStart : start + fallbackLineDuration;
            const lines = (group?.lines || []).map((line) => ({
                ...line,
                text: String(line?.text || ''),
                words: normalizeGroupWords(line?.words, start, end, line?.text)
            }));
            return { start, end, lines };
        }).filter(group => group.lines.some(line => line.text));
    }

    function parseLyrics(raw) {
        const map = new Map();
        let hasVerbatim = false;
        String(raw || '').split(/\r?\n/).forEach((rawLine) => {
            const line = rawLine.trim();
            if (!line || /^\[[A-Za-z]+:[^\]]*\]$/.test(line)) return;
            const parsed = parseLine(line);
            if (!parsed || !parsed.text) return;
            hasVerbatim = hasVerbatim || parsed.verbatim;
            if (!map.has(parsed.start)) {
                map.set(parsed.start, { start: parsed.start, lines: [] });
            }
            map.get(parsed.start).lines.push(parsed);
        });
        const result = normalizeGroups(Array.from(map.values()).sort((a, b) => a.start - b.start));
        const hasMultiLang = result.some(group => group.lines.length > 1);
        return {
            type: hasVerbatim || hasMultiLang ? 'karaoke' : 'line',
            groups: result
        };
    }

    function looksLikeRomajiLine(line) {
        const text = String(line?.text || '').trim();
        if (!text) return false;
        const latinCount = (text.match(/[A-Za-z]/g) || []).length;
        const cjkOrKanaCount = (text.match(/[\u3040-\u30ff\u3400-\u9fff]/g) || []).length;
        return latinCount > 0 && latinCount >= cjkOrKanaCount;
    }

    function splitGroupLines(lines) {
        const [orig, ...extras] = lines || [];
        let roma = null;
        let trans = null;
        extras.forEach((line) => {
            if (!roma && looksLikeRomajiLine(line)) {
                roma = line;
                return;
            }
            if (!trans) {
                trans = line;
                return;
            }
            if (!roma) {
                roma = line;
            }
        });
        return { orig, roma, trans };
    }

    function renderWords(words, fallbackStart, fallbackEnd) {
        return (words || []).map(word => [
            `<span class="desktop-lyrics-word" data-start="${word.start || fallbackStart}" data-end="${word.end || fallbackEnd}" style="--desktop-word-progress:0%;">`,
            `<span class="desktop-lyrics-word-base">${escapeHTML(word.text)}</span>`,
            `<span class="desktop-lyrics-word-fill">${escapeHTML(word.text)}</span>`,
            '</span>'
        ].join('')).join('');
    }

    function renderGroup(group, index, type) {
        if (type !== 'karaoke') {
            const firstLine = group.lines.find(line => line?.text) || group.lines[0];
            return `<div class="desktop-lyrics-group" data-index="${index}" data-start="${group.start}"><div class="desktop-lyrics-line">${escapeHTML(firstLine?.text || '')}</div></div>`;
        }
        const { orig, roma, trans } = splitGroupLines(group.lines);
        const renderLine = (line, className, useWordProgress) => {
            if (!line?.text) return '';
            const content = useWordProgress && Array.isArray(line.words) && line.words.length > 0
                ? renderWords(line.words, group.start, group.end)
                : escapeHTML(line.text);
            return `<div class="${className}">${content}</div>`;
        };
        return [
            `<div class="desktop-lyrics-group" data-index="${index}" data-start="${group.start}">`,
            renderLine(orig, 'desktop-lyrics-orig', true),
            renderLine(roma, 'desktop-lyrics-roma', !!roma?.verbatim),
            renderLine(trans, 'desktop-lyrics-trans', !!trans?.verbatim),
            '</div>'
        ].join('');
    }

    function renderLyrics(parsed) {
        groups = parsed.groups || [];
        activeIndex = -1;
        lyricsBody.innerHTML = groups.map((group, index) => renderGroup(group, index, parsed.type)).join('');
        setIdle(groups.length === 0);
    }

    async function loadLyrics(url) {
        if (!url) {
            currentLyricURL = '';
            setIdle(true);
            return;
        }
        if (url === currentLyricURL && groups.length > 0) return;
        currentLyricURL = url;
        try {
            const response = await fetch(url, { cache: 'no-store' });
            if (!response.ok) {
                setIdle(true);
                return;
            }
            renderLyrics(parseLyrics(await response.text()));
        } catch (_) {
            setIdle(true);
        }
    }

    function currentPositionMS() {
        if (!latestState) return 0;
        const base = Number(latestState.position_ms || 0);
        if (!latestState.playing) return base;
        const clientTime = Number(latestState.client_time_ms || 0);
        if (!clientTime) return base;
        return base + Math.max(0, Date.now() - clientTime);
    }

    function updateLyrics() {
        if (!latestState?.active || groups.length === 0) return;
        const ms = currentPositionMS();
        let nextIndex = -1;
        for (let i = 0; i < groups.length; i++) {
            if (ms >= groups[i].start) nextIndex = i;
            else break;
        }
        if (nextIndex < 0) return;

        const active = lyricsBody.querySelector(`.desktop-lyrics-group[data-index="${nextIndex}"]`);
        if (!active) return;
        if (nextIndex !== activeIndex) {
            lyricsBody.querySelectorAll('.desktop-lyrics-group.active').forEach(el => el.classList.remove('active'));
            active.classList.add('active');
            activeIndex = nextIndex;
        }
        lyricsBody.querySelectorAll('.desktop-lyrics-word').forEach(word => {
            const start = Number(word.dataset.start || 0);
            const end = Number(word.dataset.end || start + fallbackLineDuration);
            let progress = 0;
            if (ms > start) {
                progress = end <= start ? 1 : Math.max(0, Math.min(1, (ms - start) / (end - start)));
            }
            word.style.setProperty('--desktop-word-progress', progress === 1 ? 'calc(100% + 8px)' : `${(progress * 100).toFixed(3)}%`);
        });
    }

    function animationTick() {
        updateLyrics();
        animationFrame = requestAnimationFrame(animationTick);
    }

    async function pollState() {
        try {
            const response = await fetch(`${API_ROOT}/desktop_lyrics/state`, { cache: 'no-store' });
            if (!response.ok) return;
            const payload = await response.json();
            const state = payload?.state || {};
            latestState = state;
            if (!state.active || !state.lyric_url) {
                setIdle(true);
                return;
            }
            setIdle(false);
            await loadLyrics(state.lyric_url);
            updateLyrics();
        } catch (_) {
        }
    }

    function applyColor(color) {
        const value = /^#[0-9a-f]{6}$/i.test(String(color || '')) ? color : '#10b981';
        body.style.setProperty('--desktop-lyric-accent', value);
        if (colorInput) colorInput.value = value;
        localStorage.setItem(colorStorageKey, value);
    }

    document.getElementById('desktopLyricsClose')?.addEventListener('click', () => {
        if (typeof window.musicDlCloseDesktopLyrics === 'function') {
            window.musicDlCloseDesktopLyrics();
            return;
        }
        window.close();
    });

    document.getElementById('desktopLyricsMinimize')?.addEventListener('click', () => {
        if (typeof window.musicDlMinimizeDesktopLyrics === 'function') {
            window.musicDlMinimizeDesktopLyrics();
        }
    });

    document.getElementById('desktopLyricsMove')?.addEventListener('mousedown', (event) => {
        event.preventDefault();
        if (typeof window.musicDlMoveDesktopLyrics === 'function') {
            window.musicDlMoveDesktopLyrics();
        }
    });

    document.getElementById('desktopLyricsColor')?.addEventListener('click', () => {
        if (!colorPanel) return;
        colorPanel.hidden = !colorPanel.hidden;
    });

    document.querySelectorAll('.desktop-lyrics-swatch').forEach(button => {
        button.addEventListener('click', () => applyColor(button.dataset.color));
    });

    colorInput?.addEventListener('input', () => applyColor(colorInput.value));

    applyColor(localStorage.getItem(colorStorageKey) || '#10b981');
    setIdle(true);
    setInterval(pollState, statePollMS);
    pollState();
    animationTick();
})();
