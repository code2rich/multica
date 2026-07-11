// Asset imports — Next.js resolves these to a `StaticImageData` object with
// `.src` (plus width/height/blurDataURL). Declare the union here so
// packages/views compiles in both dev and build.
// Component code should normalise with `typeof x === "string" ? x : x.src`.
interface StaticImageAsset {
  src: string;
  height?: number;
  width?: number;
  blurDataURL?: string;
}

declare module "*.png" {
  const src: string | StaticImageAsset;
  export default src;
}
declare module "*.svg" {
  const src: string | StaticImageAsset;
  export default src;
}
