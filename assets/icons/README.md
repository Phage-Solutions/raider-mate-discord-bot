# Icons

PNGs uploaded as Discord application emoji by `make icons`. Each must be under 256KB,
which is Discord's emoji limit.

The bot looks for a spec icon first and falls back to the class, so uploading only the
13 class files is a reasonable place to stop. All 39 specs is the complete set.

Names are what Raider.IO reports, lowercased with spaces and punctuation removed. The
class prefix on a spec is not optional: Frost is a Mage and a Death Knight, Holy is a
Paladin and a Priest, Restoration is a Druid and a Shaman, Protection is a Warrior and a
Paladin.

## Artwork and licensing

This is a public AGPL repository. Blizzard's class and spec icons are their artwork, not
ours, so think before committing them here: `.gitignore` excludes `*.png` in this
directory for that reason. Point `make icons` at a local directory, or replace the
ignore rule if you have art you are entitled to redistribute.

## Filenames

```
# Death Knight
deathknight.png
deathknight_blood.png
deathknight_frost.png
deathknight_unholy.png

# Demon Hunter
demonhunter.png
demonhunter_havoc.png
demonhunter_vengeance.png

# Druid
druid.png
druid_balance.png
druid_feral.png
druid_guardian.png
druid_restoration.png

# Evoker
evoker.png
evoker_devastation.png
evoker_preservation.png
evoker_augmentation.png

# Hunter
hunter.png
hunter_beastmastery.png
hunter_marksmanship.png
hunter_survival.png

# Mage
mage.png
mage_arcane.png
mage_fire.png
mage_frost.png

# Monk
monk.png
monk_brewmaster.png
monk_mistweaver.png
monk_windwalker.png

# Paladin
paladin.png
paladin_holy.png
paladin_protection.png
paladin_retribution.png

# Priest
priest.png
priest_discipline.png
priest_holy.png
priest_shadow.png

# Rogue
rogue.png
rogue_assassination.png
rogue_outlaw.png
rogue_subtlety.png

# Shaman
shaman.png
shaman_elemental.png
shaman_enhancement.png
shaman_restoration.png

# Warlock
warlock.png
warlock_affliction.png
warlock_demonology.png
warlock_destruction.png

# Warrior
warrior.png
warrior_arms.png
warrior_fury.png
warrior_protection.png
```
