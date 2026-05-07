"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { Suspense } from "react";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

export function Breadcrumbs() {
  return (
    <Suspense fallback={<div className="h-4" />}>
      <BreadcrumbsInner />
    </Suspense>
  );
}

function BreadcrumbsInner() {
  const pathname = usePathname() || "/";
  const params = useSearchParams();
  const crumbs = derive(pathname, params);

  return (
    <Breadcrumb>
      <BreadcrumbList>
        {crumbs.map((c, i) => {
          const isLast = i === crumbs.length - 1;
          return (
            <span key={`${c.label}-${i}`} className="contents">
              <BreadcrumbItem className={i === 0 ? "hidden md:block" : undefined}>
                {isLast || !c.href ? (
                  <BreadcrumbPage>{c.label}</BreadcrumbPage>
                ) : (
                  <BreadcrumbLink asChild>
                    <Link href={c.href}>{c.label}</Link>
                  </BreadcrumbLink>
                )}
              </BreadcrumbItem>
              {!isLast && (
                <BreadcrumbSeparator
                  className={i === 0 ? "hidden md:block" : undefined}
                />
              )}
            </span>
          );
        })}
      </BreadcrumbList>
    </Breadcrumb>
  );
}

function derive(
  pathname: string,
  params: URLSearchParams | null
): { label: string; href?: string }[] {
  const id = params?.get("id");
  if (pathname === "/" || pathname === "") {
    return [{ label: "flow", href: "/" }, { label: "Flows" }];
  }
  if (pathname.startsWith("/flows/new")) {
    return [
      { label: "flow", href: "/" },
      { label: "Flows", href: "/" },
      { label: "New" },
    ];
  }
  if (pathname.startsWith("/flows/view")) {
    return [
      { label: "flow", href: "/" },
      { label: "Flows", href: "/" },
      { label: id ? `Flow ${id.slice(0, 8)}…` : "Flow" },
    ];
  }
  if (pathname.startsWith("/executions/view")) {
    return [
      { label: "flow", href: "/" },
      { label: "Executions" },
      { label: id ? `${id.slice(0, 12)}…` : "Run" },
    ];
  }
  return [{ label: "flow", href: "/" }];
}
